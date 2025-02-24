
def is_valid_login_code(code: str) -> bool:
    """
    Check if the given string is a valid Microsoft Entra ID login code.

    A valid login code must be exactly 9 characters long and consist of
    uppercase letters and digits.

    :param code: The login code to check.
    :return: True if the code is valid, False otherwise.
    """
    return len(code) == 9 and code.isalnum() and code.isupper()
