Feature: AppArmor Prompting
  Test the AppArmor prompting feature

  Background:
    Given I have an Ubuntu Desktop machine with the desktop-security-center installed
    And I logged in

  Scenario: Enabling prompting in the Security Center
      When I launch the Security Center
