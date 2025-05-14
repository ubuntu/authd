Feature: authd GDM login
  Test logging in with authd via GDM

  Background:
    Given I have an Ubuntu Desktop machine set up to test authd and booted to the GDM login screen
    And I have a second machine with a web browser

  Scenario: First login (MS Entra ID)
    When I enter the username of the test user
    Then I am asked to select the broker
    When I select the "Microsoft Entra ID" broker
    Then I see the message "Scan the QR code or access "https://microsoft.com/devicelogin" and use the provided login code"
    And I see a QR code which encodes the URL "https://microsoft.com/devicelogin"
    And I see a valid Microsoft Entra ID login code
    When I open "https://microsoft.com/devicelogin" on the second machine and log in
#    And I enter the login code "user_code"
#    And I log in with the username "demo@uaadtest.onmicrosoft.com" and password "password"
#    Then I am asked if I am trying to sign in to "Azure OIDC Poc"
#    When I click "Continue"
#    Then I am prompted to create a local password
#    When I enter a password
#    And confirm the password
#    Then I am logged in
