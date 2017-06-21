
import UserInviteElement from './pageobjects/user_invite.element';

describe('User roles', function () {
  let userInviteElement;

  const email = 'fake-new-user@domain.com';
  const urlOrgX = '/#/org/48b3f8a1-ffe7-4aa8-8e85-94768d6bd250';

  describe('A user on page for an org', function () {
    it('should navigates to org X', function () {
      browser.url(urlOrgX);
      browser.waitForExist('.test-users');
    });

    it('should have the user invite panel', function () {
      browser.url(urlOrgX);

      browser.waitForExist('.test-users-invite');
      userInviteElement = new UserInviteElement(browser,
        browser.element(UserInviteElement.primarySelector));

      expect(userInviteElement.isVisible()).toBe(true);
    });

    it('should be able to input content into invite form', function () {
      userInviteElement.inputToInviteForm(email);

      expect(email).toBe(userInviteElement.getInviteFormValue());
    });

    it('should be able to submit an email address and see on user list', function () {
      const existingUserCount = userInviteElement.countNumberOfUsers();
      let user = userInviteElement.getUserByIndex(existingUserCount - 1);
      expect(user.getText()).not.toMatch(/fake-new-user@domain.com/);
      userInviteElement.inputToInviteForm(email);
      userInviteElement.submitInviteForm();
      const currentUserCount = userInviteElement.countNumberOfUsers();
      expect(currentUserCount).toEqual(existingUserCount + 1);
      user = userInviteElement.getUserByIndex(currentUserCount - 1);
      // TODO disabling test due to changing functionality
      // expect(user.getText()).toMatch(/fake-new-user@domain.com/);
    });

    it('should display an error message if the email address is invalid',
    function () {
      const invalidEmail = '123';
      userInviteElement.inputToInviteForm(invalidEmail);
      userInviteElement.submitInviteForm();
      const topError = 'There were errors submitting the form.';
      const bottomError = 'The value entered in email is not a valid e-mail address';
      expect(userInviteElement.getErrorMessage()).toMatch(topError);
      expect(userInviteElement.getValidatorMessage()).toMatch(bottomError);
    });
  });
});
