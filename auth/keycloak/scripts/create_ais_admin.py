import argparse
import sys
from getpass import getpass

from keycloak import KeycloakAdmin


def prompt_password_with_confirmation(username) -> str:
    while True:
        pwd = getpass(f"Password for new user '{username}': ")
        pwd_confirm = getpass("Confirm password: ")
        if pwd != pwd_confirm:
            print("Passwords do not match, please try again.", file=sys.stderr)
            continue
        if not pwd:
            print("Password cannot be empty, please try again.", file=sys.stderr)
            continue
        return pwd


def parse_args():
    parser = argparse.ArgumentParser(
        description="Create 'ais-admin' user in a Keycloak realm."
    )
    parser.add_argument(
        "--host",
        required=True,
        help="Keycloak base URL including port, e.g. https://localhost:8543/",
    )
    parser.add_argument(
        "--realm",
        required=True,
        help="Target realm where 'ais-admin' will be created.",
    )
    parser.add_argument(
        "--admin-user",
        required=True,
        help="Keycloak admin username (in master realm).",
    )
    parser.add_argument(
        "--admin-pass",
        required=True,
        help="Keycloak admin password (in master realm).",
    )
    parser.add_argument(
        "--insecure",
        action="store_true",
        help="Disable SSL verification (use only for testing).",
    )
    parser.add_argument(
        "--verify-ca",
        metavar="CA_FILE",
        help=(
            "Path to CA certificate bundle to use for TLS verification. "
            "Takes precedence over --insecure."
        ),
    )
    return parser.parse_args()


def _build_verify_option(args):
    """
    Decide what to pass to python-keycloak's 'verify' parameter.

    - If --verify-ca is set, return that path.
    - Else if --insecure is set, return False.
    - Else return True (default TLS verification).
    """
    if args.verify_ca:
        return args.verify_ca
    if args.insecure:
        return False
    return True


def create_admin(args):
    verify_opt = _build_verify_option(args)

    try:
        return KeycloakAdmin(
            server_url=args.host,
            username=args.admin_user,
            password=args.admin_pass,
            realm_name=args.realm,       # target realm for admin operations
            user_realm_name="master",    # credentials are in master realm
            verify=verify_opt,           # bool or CA bundle path
        )
    except Exception as e:
        print(f"Failed to create Keycloak admin client: {e}", file=sys.stderr)
        sys.exit(1)


def check_user_existence(admin, realm, user) -> bool:
    try:
        existing_users = admin.get_users({"username": user})
        if existing_users:
            print(
                "User 'ais-admin' already exists in realm "
                f"'{realm}' (id={existing_users[0].get('id')})."
            )
            return True
        return False
    except Exception as e:
        print(f"Error while checking for existing user: {e}", file=sys.stderr)
        sys.exit(1)


def create_realm_user(admin, realm, user):
    user_password = prompt_password_with_confirmation(user)

    user_representation = {
        "username": user,
        "enabled": True,
        "credentials": [
            {
                "type": "password",
                "value": user_password,
                "temporary": False,
            }
        ],
    }

    try:
        user_id = admin.create_user(user_representation, exist_ok=True)
        if not user_id:
            users = admin.get_users({"username": user})
            user_id = users[0]["id"] if users else None

        print(
            f"User '{user}' created in realm '{realm}'"
            + (f" with id={user_id}" if user_id else "")
        )
    except Exception as e:
        print(f"Failed to create user '{user}': {e}", file=sys.stderr)
        sys.exit(1)

def join_admin_group(admin, realm, user):
    group_name = "Admin Users"
    user_id = admin.get_user_id(user)
    group = admin.get_group_by_path(f"/{group_name}")
    group_id = group["id"]
    try:
        admin.group_user_add(user_id=user_id, group_id=group_id)
        print(f"User '{user}' added to group '{group_name}' in realm '{realm}'")
    except Exception as e:
        print(f"Failed to add '{user}' to group '{group_name}': {e}", file=sys.stderr)
        sys.exit(1)


def main():
    args = parse_args()
    admin = create_admin(args)
    realm = args.realm
    user = "ais-admin"
    exists = check_user_existence(admin, realm, user)
    if not exists:
        create_realm_user(admin, realm, user)
    join_admin_group(admin, realm, user)


if __name__ == "__main__":
    main()
