Feature: authd GDM login
  Test logging in with authd via GDM

  Background:
    Given I started Ubuntu Desktop
    And I installed the authd package

  Scenario: First login (MS Entra ID)
    Given I installed the authd-msentraid snap
    When I click on "Not listed?"
    And I enter the UPN of the test user in the "Username" field
    And I press "Enter"
    Then I am asked to select the broker
    When I select the "Microsoft Entra ID" broker
    Then I see the message "Scan the QR code or access "https://microsoft.com/devicelogin" and use the provided login code"
    And I see a QR code
    And I see a login code
    When I open "https://microsoft.com/devicelogin" on another machine and log in
#    And I enter the login code "user_code"
#    And I log in with the username "demo@uaadtest.onmicrosoft.com" and password "password"
#    Then I am asked if I am trying to sign in to "Azure OIDC Poc"
#    When I click "Continue"
    Then I am prompted to create a local password
    When I enter a password
    And confirm the password
    Then I am logged in

