package helpers

import (
	"crypto/tls"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/boj/redistore"
	"github.com/cloudfoundry-community/go-cfenv"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/sessions"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const (
	// 7 days at most.
	expirationConstant = 60 * 60 * 24 * 7
)

// Settings is the object to hold global values and objects for the service.
type Settings struct {
	// OAuthConfig is the OAuth client with all the parameters to talk with CF's UAA OAuth Provider.
	OAuthConfig *oauth2.Config
	// Console API
	ConsoleAPI string
	// Login URL - used to redirect users to the logout page
	LoginURL string
	// Sessions is the session store for all connected users.
	Sessions sessions.Store
	// Generate secure random state
	StateGenerator func() (string, error)
	// UAA API
	UaaURL string
	// Log API
	LogURL string
	// Path to root of project.
	BasePath string
	// High Privileged OauthConfig
	HighPrivilegedOauthConfig *clientcredentials.Config
	// A flag to indicate whether profiling should be included (debug purposes).
	PProfEnabled bool
	// Build Info
	BuildInfo string
	// Set the secure flag on session cookies
	SecureCookies bool
	// Inidicates if targeting a local CF environment.
	LocalCF bool
	// URL where this app is hosted
	AppURL string
	// Type of session backend
	SessionBackend string
	// Returns whether the backend is up.
	SessionBackendHealthCheck func() bool
	// SMTP host for UAA invites
	SMTPHost string
	// SMTP post for UAA invites
	SMTPPort string
	// SMTP user for UAA invites
	SMTPUser string
	// SMTP password for UAA invites
	SMTPPass string
	// SMTP from address for UAA invites
	SMTPFrom string
	// Shared secret with CF API proxy
	TICSecret string
}

// CreateContext returns a new context to be used for http connections.
func (s *Settings) CreateContext() context.Context {
	ctx := context.TODO()
	// If targeting local cf env, we won't have
	// valid SSL certs so we need to disable verifying them.
	if s.LocalCF {
		httpClient := http.DefaultClient
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	}
	return ctx
}

// InitSettings attempts to populate all the fields of the Settings struct. It will return an error if it fails,
// otherwise it returns nil for success.
func (s *Settings) InitSettings(envVars *EnvVars, env *cfenv.App) (retErr error) {
	defer func() {
		// While .MustGet() is convenient in readability below, we'd prefer to convert this
		// to an error for upstream callers.
		if r := recover(); r != nil {
			missingErr, ok := r.(*ErrMissingEnvVar)
			if !ok {
				// We don't know what this is, re-panic
				panic(r)
			}

			// Set return code to the actual error
			retErr = missingErr
		}
	}()

	s.BasePath = envVars.Get(BasePathEnvVar, "")
	s.AppURL = envVars.MustGet(HostnameEnvVar)
	s.ConsoleAPI = envVars.MustGet(APIURLEnvVar)
	s.LoginURL = envVars.MustGet(LoginURLEnvVar)
	s.UaaURL = envVars.MustGet(UAAURLEnvVar)
	s.LogURL = envVars.MustGet(LogURLEnvVar)
	s.PProfEnabled = envVars.BoolGet(PProfEnabledEnvVar)
	s.BuildInfo = envVars.Get(BuildInfoEnvVar, "developer-build")
	s.LocalCF = envVars.BoolGet(LocalCFEnvVar)
	s.SecureCookies = envVars.BoolGet(SecureCookiesEnvVar)
	// Safe guard: shouldn't run with insecure cookies if we are
	// in a non-development environment (i.e. production)
	if s.LocalCF == false && s.SecureCookies == false {
		return errors.New("cannot run with insecure cookies when targeting a production CF environment")
	}

	// Setup OAuth2 Client Service.
	s.OAuthConfig = &oauth2.Config{
		ClientID:     envVars.MustGet(ClientIDEnvVar),
		ClientSecret: envVars.MustGet(ClientSecretEnvVar),
		RedirectURL:  s.AppURL + "/oauth2callback",
		Scopes:       []string{"cloud_controller.read", "cloud_controller.write", "cloud_controller.admin", "scim.read", "openid"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  envVars.MustGet(LoginURLEnvVar) + "/oauth/authorize",
			TokenURL: envVars.MustGet(UAAURLEnvVar) + "/oauth/token",
		},
	}

	s.StateGenerator = func() (string, error) {
		return GenerateRandomString(32)
	}

	// Initialize Sessions.
	switch envVars.Get(SessionBackendEnvVar, "") {
	case "redis":
		address, password, err := getRedisSettings(env)
		if err != nil {
			return err
		}
		// Create a common redis pool of connections.
		redisPool := &redis.Pool{
			MaxIdle:     10,
			IdleTimeout: 240 * time.Second,
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, pingErr := c.Do("PING")
				return pingErr
			},
			Dial: func() (redis.Conn, error) {
				// We need to control how long connections are attempted.
				// Currently will limit how long redis should respond back to
				// 10 seconds. Any time less than the overall connection timeout of 60
				// seconds is good.
				c, dialErr := redis.Dial("tcp", address,
					redis.DialConnectTimeout(10*time.Second),
					redis.DialWriteTimeout(10*time.Second),
					redis.DialReadTimeout(10*time.Second))
				if dialErr != nil {
					return nil, dialErr
				}
				if password != "" {
					if _, authErr := c.Do("AUTH", password); err != nil {
						c.Close()
						return nil, authErr
					}
				}
				return c, nil
			},
		}
		// create our redis pool.
		store, err := redistore.NewRediStoreWithPool(redisPool, []byte(envVars.MustGet(SessionKeyEnvVar)))
		if err != nil {
			return err
		}
		store.SetMaxLength(4096 * 4)
		store.Options = &sessions.Options{
			HttpOnly: true,
			MaxAge:   expirationConstant,
			Path:     "/",
			Secure:   s.SecureCookies,
		}
		s.Sessions = store
		s.SessionBackend = "redis"

		// Use health check function where we do a PING.
		s.SessionBackendHealthCheck = func() bool {
			c := redisPool.Get()
			defer c.Close()
			_, err := c.Do("PING")
			if err != nil {
				log.Printf("{\"health-check-error\": \"%s\"}", err)
				return false
			}
			return true
		}
	default:
		store := sessions.NewFilesystemStore("", []byte(envVars.MustGet(SessionKeyEnvVar)))
		store.MaxLength(4096 * 4)
		store.Options = &sessions.Options{
			HttpOnly: true,
			// TODO remove this; work-around for
			// https://github.com/gorilla/sessions/issues/96
			MaxAge: expirationConstant,
			Path:   "/",
			Secure: s.SecureCookies,
		}
		s.Sessions = store
		s.SessionBackend = "file"
		s.SessionBackendHealthCheck = func() bool { return true }
	}

	// Want to save a struct into the session. Have to register it.
	gob.Register(oauth2.Token{})

	s.HighPrivilegedOauthConfig = &clientcredentials.Config{
		ClientID:     envVars.MustGet(ClientIDEnvVar),
		ClientSecret: envVars.MustGet(ClientSecretEnvVar),
		Scopes:       []string{"scim.invite", "cloud_controller.admin", "scim.read"},
		TokenURL:     envVars.MustGet(UAAURLEnvVar) + "/oauth/token",
	}

	s.SMTPFrom = envVars.MustGet(SMTPFromEnvVar)
	s.SMTPHost = envVars.MustGet(SMTPHostEnvVar)
	s.SMTPPass = envVars.Get(SMTPPassEnvVar, "")
	s.SMTPPort = envVars.Get(SMTPPortEnvVar, "")
	s.SMTPUser = envVars.Get(SMTPUserEnvVar, "")
	s.TICSecret = envVars.Get(TICSecretEnvVar, "")
	return nil
}

func getRedisSettings(env *cfenv.App) (string, string, error) {
	var err error
	// Try to read directly from REDIS_URI first.
	uri := os.Getenv("REDIS_URI")
	if uri == "" {
		// If no direct REDIS_URI, parse VCAP_SERVICES
		uri, err = getRedisService(env)
	}
	// If nothing worked so far, default to localhost
	if uri == "" || err != nil {
		uri = "redis://localhost:6379"
	}

	u, err := url.Parse(uri)
	if err != nil {
		return "", "", err
	}

	password := ""
	if u.User != nil {
		password, _ = u.User.Password()
	}

	return u.Host, password, nil
}

func getRedisService(env *cfenv.App) (string, error) {
	if env == nil {
		return "", errors.New("Empty Cloud Foundry environment")
	}
	services, err := env.Services.WithTag("redis")
	if err != nil {
		return "", err
	}
	if len(services) == 0 {
		return "", errors.New(`Could not find service with tag "redis"`)
	}
	uri, ok := services[0].Credentials["uri"].(string)
	if !ok {
		if uri, err = getRedisURIFromParts(services[0]); err == nil {
			return uri, nil
		}
		return "", errors.New("Could not parse redis uri")
	}
	return uri, nil
}

// TODO: Delete after east-west is retired
func getRedisURIFromParts(service cfenv.Service) (string, error) {
	host, ok := service.Credentials["hostname"].(string)
	if !ok {
		return "", errors.New(`Could not find "host" key`)
	}

	port, ok := service.Credentials["port"].(string)
	if !ok {
		return "", errors.New(`Could not find "port" key`)
	}

	password, ok := service.Credentials["password"].(string)
	if !ok {
		return "", errors.New(`Could not find "password" key`)
	}

	return fmt.Sprintf("redis://:%s@%s:%s", password, host, port), nil
}
