# Local authentication

Local authentication supports self-hosted browser login and a controlled break-glass path. Local users are administrator-created; LibreDash does not provide public self-registration.

## Enable the mode

Configure local auth with production security requirements:

```sh
LIBREDASH_PRODUCTION=true
LIBREDASH_LOCAL_AUTH=true
LIBREDASH_CSRF_KEY=<at-least-32-character-random-secret>
LIBREDASH_COOKIE_SECURE=true
LIBREDASH_ALLOWED_HOSTS=dash.example.com
```

Validate the complete environment:

```sh
libredash config validate --production
```

The CSRF key protects CSRF state and OAuth state cookies. Store it in the deployment secret manager. Rotating it can invalidate security state and should follow a controlled maintenance procedure.

## Bootstrap the first administrator

Production provisioning may set `LIBREDASH_BOOTSTRAP_ADMIN_EMAIL` and run:

```sh
libredash admin bootstrap
```

The bootstrap workflow creates an owner principal and API token needed to establish normal administration. Treat bootstrap material as one-time recovery-sensitive output: capture it through a protected channel, create ordinary scoped administrative identities, and retire unrestricted bootstrap credentials.

The supported Hetzner deployment wraps this process and emits a forced-change temporary password plus a limited publisher token through a one-time command.

## Create local users

A principal with grant-management authority can create a local user through the Admin / Principals surface or `POST /api/v1/principals`. The response returns a temporary password once. Deliver it out of band and require the user to replace it at first sign-in.

Do not place temporary passwords in tickets, chat rooms, shell history, deployment output retained broadly, or automation variables. If delivery is uncertain, reset the password instead of forwarding the same value again.

## Reset access

Use `POST /api/v1/principals/{principal}/password-reset` to issue a new temporary credential. LibreDash never reveals the previous password. A reset should force a password change and produce an audit event.

When a principal is no longer trusted, deactivate the principal and revoke sessions and API tokens rather than only resetting the password. Review direct grants and group membership as part of offboarding.

## Authorization behavior

Local users map to ordinary user principals. They use the same sessions, roles, grants, API tokens, data policies, and audit events as OIDC or SCIM identities. Authentication proves the principal; it does not grant workspace access by itself.

Local workspace groups use `provider: local` in the runtime access model and can be represented in project access resources. Prefer group-based workspace bindings for teams and direct user grants only for exceptions.

## Operate a break-glass path

If policy requires emergency local access alongside OIDC:

- keep the account individually attributable;
- store recovery material in an access-controlled vault;
- test sign-in and required privileges on a schedule;
- alert on any use;
- rotate the password or token after an incident;
- review all actions in the audit trail;
- do not embed the credential in automated health checks.

A shared permanent owner password is not a break-glass design. It removes attribution and tends to escape controlled storage.

## Review checklist

Confirm secure cookies, allowed hosts, TLS, rate limiting at the trusted edge, strong secrets, temporary-password delivery, session revocation, and audit retention. Use [Authentication and authorization](/docs/enterprise-auth), [Roles, grants, and policies](/docs/security/authorization), and the generated [environment reference](/docs/configuration).
