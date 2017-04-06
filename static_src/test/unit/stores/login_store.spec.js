
import '../../global_setup.js';

import AppDispatcher from '../../../dispatcher.js';
import { loginActionTypes } from '../../../constants';
import { LoginStore as _LoginStore } from '../../../stores/login_store';

describe('LoginStore', () => {
  let sandbox, LoginStore;

  beforeEach(() => {
    sandbox = sinon.sandbox.create();
    LoginStore = new _LoginStore();
  });

  afterEach(() => {
    sandbox.restore();
  });

  describe('constructor()', () => {
    it('should set isLoggedIn to true', () => {
      expect(LoginStore.isLoggedIn()).toBe(true);
    });
  });

  describe('on received status', function () {
    let authStatus;

    beforeEach(function () {
      authStatus = { status: 'authorized' };
      sandbox.stub(LoginStore, 'emitChange');

      AppDispatcher.handleServerAction({
        type: loginActionTypes.RECEIVED_STATUS,
        authStatus
      });
    });

    it('should emit a change event', function () {
      expect(LoginStore.emitChange).toHaveBeenCalledOnce();
    });

    it('should set isLoggedIn to true if logged in', function () {
      expect(LoginStore.isLoggedIn()).toBe(true);
    });

    describe('given unauthorized', function () {
      beforeEach(function () {
        AppDispatcher.handleServerAction({
          type: loginActionTypes.RECEIVED_STATUS,
          authStatus: { status: 'unauthorized' }
        });
      });

      it('should set authenticated to false', function () {
        expect(LoginStore.isLoggedIn()).toBe(false);
      });
    });
  });

  describe('FETCH_STATUS', function () {
    beforeEach(function () {
      LoginStore._error = new Error('error');

      AppDispatcher.handleViewAction({
        type: loginActionTypes.FETCH_STATUS
      });
    });

    it('clears errors', function () {
      expect(LoginStore._error).toBe(null);
      expect(LoginStore.error).toBe(null);
    });
  });

  describe('ERROR_STATUS', function () {
    let err;
    beforeEach(function () {
      err = new Error('error');
      LoginStore._isAuthenticated = 'authorized';

      AppDispatcher.handleViewAction({
        type: loginActionTypes.ERROR_STATUS,
        err
      });
    });

    it('sets error', function () {
      expect(LoginStore._error).toBe(err);
      expect(LoginStore.error).toBe(err);
    });

    it('does not affect isLoggedIn', function () {
      expect(LoginStore.isLoggedIn()).toBe(true);
      expect(LoginStore._isAuthenticated).toBe('authorized');
    });
  });
});
