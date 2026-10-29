# The development realm

`ensoria-realm.json` is imported into Keycloak on its first start by the
`ensoria-ensoria-template-keycloak` service in `compose.yaml`. It exists so that
the application can be developed against **real tokens from a real issuer**,
which `AUTH_MODE=hs256` cannot show.

It is a disposable development realm and is not a starting point for a
production one: every secret in it is public, the user's password is written
down in it, and its lifetimes are chosen for convenience. A real deployment gets
its realm from whoever runs its identity provider.

Realm JSON has no comment syntax — Keycloak refuses a file containing a field it
does not recognise, `_comment` included — so what would have been comments is
here instead.

## ⚠ You have to keep the scope list in step with your code

**This is the one part of the file a project must maintain.** The client scopes
named `users:read`, `orders:read`, `worker.jobs:write` and so on are the
permissions *this template's example endpoints and WebSocket messages* declare:

```go
Security: &restkit.SecuritySpec{Scopes: []string{"users:read"}},
Security: &wskit.MessageSecurity{Scopes: []string{"announcements:write"}},
```

They are not framework vocabulary and nothing derives them from the code. When
you delete the example modules, or declare a scope of your own, the realm still
offers the old list — and a token cannot carry a scope the realm has never heard
of, so the endpoint answers `403` with nothing in the logs to explain it.

To add one, add a client scope and list it under the client's
`optionalClientScopes`:

```json
{
  "name": "invoices:write",
  "description": "Issue an invoice",
  "protocol": "openid-connect",
  "attributes": {
    "include.in.token.scope": "true",
    "display.on.consent.screen": "false"
  }
}
```

`include.in.token.scope: true` is what puts the name into the token's `scope`
claim. Without it the scope exists and is granted and never reaches the
application.

Then re-import (see [Changing it](#changing-it)) — editing the file alone
changes nothing.

## What is in it

| Thing | Value | Why |
|---|---|---|
| Realm | `ensoria` | The issuer becomes `http://localhost:8081/realms/ensoria` |
| Client | `ensoria-frontend` | Public client. Standard flow for a browser on `http://localhost:3000`, direct access grants so `curl` can get a token in one request |
| User | `alice` / `alice` | The only account. It may request every scope |
| Client scope `basic` | The `sub` mapper | Puts the user's id in `sub` |
| Client scopes `users:read`, … | One per declared permission | Requested per token, so `403` can be demonstrated as easily as `200` |
| User attribute `approval_limit` = 1500 | The `approval-limit` mapper | A claim `POST /order/{id}/approval` reads. 1500 covers order 1 and not order 2, so both answers can be seen |

## Three things Keycloak's defaults get wrong for this application

Both were found by running it, and both fail in a way that produces a working
token the application then refuses — or accepts while authorizing nothing.

**The audience.** `AUTH_AUDIENCE` is checked against the token's `aud` when it is
set, and Keycloak's default `aud` is `account`. Every token would be refused. The
client carries an `ensoria-audience` mapper that adds `ensoria`. Leaving
`AUTH_AUDIENCE` unset skips the check instead — but then any token this issuer
signed is accepted, including one minted for a different application in the same
realm.

**Where the permissions go.** The application reads them from the
space-separated `scope` claim (RFC 8693), which is where an OAuth 2 access token
carries them. It is tempting to model permissions as **realm roles** instead —
that is the first thing the admin console suggests — but Keycloak puts those in
`realm_access.roles`, which nothing here reads. Mapping the roles into `scope`
does not fix it either: that mapper writes a **JSON array**, and the application
reads a string, so the claim is silently ignored and the token authorizes
nothing. Modelling permissions as client scopes is both the fix and the correct
OAuth modelling.

**The type of a claim.** A claim carrying a number has to arrive as a number.
Keycloak's attribute mappers write **strings** unless `jsonType.label` says
otherwise, and `authkit.Claims.Number` refuses a string on purpose — guessing
that `"1500"` meant 1500 would be this application inventing an authorization
decision the issuer never made. The `approval-limit` mapper therefore sets
`"jsonType.label": "long"`:

```json
{
  "name": "approval-limit",
  "protocolMapper": "oidc-usermodel-attribute-mapper",
  "config": {
    "user.attribute": "approval_limit",
    "claim.name": "approval_limit",
    "jsonType.label": "long",
    "access.token.claim": "true"
  }
}
```

Without it the token still verifies, the caller is still authenticated, and the
approval is refused with a `403` that says nothing about why. `encli auth token
--claim-json approval_limit=1500` mints the correct shape locally, and
`--claim approval_limit=1500` mints the broken one, if you want to see both
without touching a realm.

If you point the application at a realm somebody else configured, these are the
three things to check first.

## Why the built-in client scopes are written out

Declaring `clientScopes` at the realm level **replaces** Keycloak's built-in set
rather than adding to it. That is why `basic` appears here spelled out: without
it there is no `sub` mapper, and tokens arrive with no subject at all — which
surfaces much later, as sessions belonging to nobody.

Only what this application needs is declared. `profile`, `email`, `roles` and
`web-origins` are absent because nothing reads them; add them back if you start
to.

## Changing it

Keycloak imports this file **once**, into its own storage. Editing it afterwards
changes nothing. To import again:

```bash
docker compose --profile keycloak down -v   # -v drops the realm storage
docker compose --profile keycloak up -d
```

Changes made through the admin console (http://localhost:8081, `admin` /
`admin`) live in that storage and are lost the same way. Export them back into
this file if they are worth keeping.
