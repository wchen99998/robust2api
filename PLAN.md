# Control-Local Identity Refactor Spec and Action Plan

Status: draft scratch plan
Updated: 2026-04-10
Scope: `control` runtime only. `gateway` and `worker` remain separate runtimes.

This file is a temporary working spec and execution plan for the control-local identity refactor. It translates the proposed target architecture into repo-specific work items for this codebase.

## 1. Goals

- Turn `control` into a modular monolith with clear internal domains: `identity`, `registration`, `access`, `provideraccounts`, and `commerce`.
- Replace the current header-token and localStorage browser auth model with cookie-backed browser sessions.
- Replace the scattered frontend auth/profile/settings surface with a compact BFF API centered on `bootstrap`, `session`, `registration`, `password`, `oauth`, and `me`.
- Introduce short-lived ES256 access JWTs plus server-side session validation so revocation is immediate.
- Remove the admin API key auth path entirely. Admin access becomes normal subject-based auth plus role checks.
- Keep `gateway` isolated from end-user auth. Gateway stays API-key-only.

## 2. Non-goals

- No separate identity binary in this phase.
- No change to `gateway` request auth, quota enforcement, or provider token selection semantics.
- No rewrite of provider-account OAuth onboarding or worker token refresh beyond keeping them operational.
- No machine-client replacement for the removed admin API key in this phase.
- No multi-database split. Postgres and Redis stay shared deployments.

## 3. Current-State Inventory

The current implementation is still organized around a monolithic auth service and a split frontend auth surface.

### Backend touchpoints

- `backend/internal/service/auth_service.go`
  - Owns login, registration, refresh, forgot/reset password, pending OAuth registration, Turnstile verification, and token issuance.
- `backend/internal/service/user_service.go`
  - Still owns password change and profile update.
- `backend/internal/server/routes/auth.go`
  - Registers `/auth/*`, `/settings/public`, and `/auth/me`.
- `backend/internal/server/routes/user.go`
  - Registers `/user/profile`, `/user/password`, and `/user/totp/*`.
- `backend/internal/server/middleware/jwt_auth.go`
  - Reads bearer JWT from `Authorization`, validates it locally, then reloads the user and compares `TokenVersion`.
- `backend/internal/server/middleware/admin_auth.go`
  - Supports both admin JWT auth and `x-api-key` admin auth.
- `backend/internal/server/middleware/backend_mode_guard.go`
  - Hardcodes the legacy auth route names that remain usable in backend-only mode.
- `backend/internal/handler/auth_handler.go`
  - Returns access and refresh tokens in JSON.
- `backend/internal/handler/auth_linuxdo_oauth.go`
  - Redirects back to the SPA with tokens or pending OAuth state in the URL fragment.
- `backend/internal/handler/auth_oidc_oauth.go`
  - Same fragment-token pattern as LinuxDo.
- `backend/internal/server/routes/admin.go`
  - Exposes admin settings routes for the admin API key.
- `backend/internal/handler/admin/setting_handler.go`
  - Implements admin API key status/regenerate/delete.

### Frontend touchpoints

- `frontend/src/api/auth.ts`
  - Stores access and refresh tokens in localStorage.
  - Calls `/auth/*`, `/settings/public`, and `/auth/me`.
- `frontend/src/api/totp.ts`
  - Calls `/user/totp/*`.
- `frontend/src/api/client.ts`
  - Injects `Authorization: Bearer ...` from localStorage.
  - Implements token refresh by posting refresh tokens from localStorage.
- `frontend/src/stores/auth.ts`
  - Restores session from localStorage and treats JWT presence as auth state.
- `frontend/src/views/auth/LinuxDoCallbackView.vue`
  - Reads access/refresh tokens from URL fragments and stores them in localStorage.
- `frontend/src/views/auth/OidcCallbackView.vue`
  - Same fragment-token model as LinuxDo.
- `frontend/src/api/admin/settings.ts`
  - Exposes admin API key settings UI calls that will need to be removed.

### Existing naming collision to avoid

- `backend/internal/service/identity_service.go` already exists and is unrelated to end-user auth. It fingerprints upstream provider-account traffic.
- Do not add a second `IdentityService` under `backend/internal/service`. The new auth "identity" domain should live under a dedicated `backend/internal/controlplane/identity` package or equivalent.

## 4. Target Architecture

### 4.1 Runtime and domain boundaries

`control` stays a single binary and a single HTTP surface, but internal ownership becomes explicit:

- `identity`
  - Subjects, password credentials, federated identities, sessions, refresh rotation, MFA, JWT signing, JWKS.
- `registration`
  - Signup policy evaluation, invitation/promo/email-policy checks, initial provisioning, first session creation.
- `access`
  - Application roles and authorization checks. Roles are not embedded in JWTs.
- `provideraccounts`
  - Upstream Claude/OpenAI/Gemini/Antigravity OAuth and credential lifecycle for admin onboarding and worker refresh.
- `commerce`
  - API keys, groups, subscriptions, balances, billing, usage, and related control-plane self-service/admin features.

`gateway` remains independent from browser auth and session state.

### 4.2 Proposed package layout

The current repo already has `backend/internal/controlplane/provider.go`. Use that namespace for the new modular monolith instead of growing `backend/internal/service/auth_service.go` further.

Proposed structure:

```text
backend/internal/controlplane/
  identity/
    service.go
    session_service.go
    refresh_service.go
    password_service.go
    mfa_service.go
    jwt_service.go
    cookie_service.go
    repository.go
  registration/
    service.go
    preflight.go
    repository.go
  access/
    service.go
    middleware.go
    repository.go
  provideraccounts/
    facade.go
  commerce/
    facade.go
```

Proposed control-plane handlers:

```text
backend/internal/handler/
  bootstrap_handler.go
  session_handler.go
  registration_handler.go
  password_handler.go
  me_handler.go
  oauth_handler.go
```

This allows `ControlHandlers` to stop centering everything on `Auth`, `User`, and `Totp`.

### 4.3 Repo-specific transition model

The biggest practical constraint is that the current commerce layer is keyed by `users.id` across API keys, usage, subscriptions, redeem codes, and admin flows.

The refactor should not try to rewrite every commerce table to UUID in one pass.

Recommended transition model:

- Introduce `subject_id UUID` as the canonical auth identity.
- Keep the existing numeric `users.id` as the current commerce key during this refactor.
- Add a stable 1:1 mapping between `subject_id` and legacy `users.id`.
- Resolve `subject_id -> legacy user_id` inside control middleware or a small resolver service when existing commerce services still expect numeric user IDs.
- Move new auth state out of `users` even if many business tables still point at `users.id`.

This gives the refactor a clean auth boundary without forcing a full commerce re-key migration in the same branch.

## 5. Data Model

### 5.1 Target tables

Auth-side tables:

- `auth_subjects`
- `auth_password_credentials`
- `auth_federated_identities`
- `auth_sessions`
- `auth_refresh_tokens`
- `auth_mfa_totp_factors`
- `auth_email_verifications`
- `auth_password_reset_tokens`
- `auth_flows`
- `auth_registration_challenges`

Control-side tables:

- `control_user_profiles`
- `control_subject_roles`

### 5.2 Table responsibilities

Recommended columns and semantics:

- `auth_subjects`
  - `id UUID PK`
  - `legacy_user_id BIGINT UNIQUE NULL` during transition
  - `primary_email`
  - `status`
  - `auth_version BIGINT`
  - timestamps
- `auth_password_credentials`
  - `subject_id`
  - `password_hash`
  - `password_changed_at`
- `auth_federated_identities`
  - `subject_id`
  - `provider`
  - `issuer`
  - `external_subject`
  - optional display metadata from provider
  - unique on `(provider, issuer, external_subject)`
- `auth_sessions`
  - `id UUID PK`
  - `subject_id`
  - `status`
  - `amr`
  - `last_seen_at`
  - `expires_at`
  - `revoked_at`
  - `current_refresh_token_hash`
- `auth_refresh_tokens`
  - `id UUID PK`
  - `session_id`
  - `token_hash`
  - `replaced_by_token_id`
  - `used_at`
  - `revoked_at`
  - `expires_at`
- `auth_mfa_totp_factors`
  - `subject_id`
  - `secret_encrypted`
  - `enabled_at`
  - `disabled_at`
  - verification mode metadata
- `auth_email_verifications`
  - `purpose`
  - `email`
  - `code_hash`
  - `expires_at`
  - `consumed_at`
- `auth_password_reset_tokens`
  - `subject_id`
  - `email`
  - `token_hash`
  - `expires_at`
  - `consumed_at`
- `auth_flows`
  - generic short-lived server-side flow state
  - OAuth `state`, PKCE verifier, redirect target, nonce, or login challenge payload
- `auth_registration_challenges`
  - pending OAuth registration state after callback when invitation is still required
- `control_user_profiles`
  - `subject_id UUID PK`
  - `legacy_user_id BIGINT UNIQUE NULL` during transition
  - username, notes, and other editable profile fields
- `control_subject_roles`
  - `subject_id`
  - `role`
  - timestamps
  - unique active role per subject and role value

### 5.3 Existing `users` table migration

Current `users` holds:

- password hash
- role
- status
- balance
- concurrency
- TOTP state
- username and notes

Planned split:

- Move auth data out of `users`:
  - password hash
  - account status for auth gating
  - token version replacement (`auth_version`)
  - TOTP state
  - federated identity linkage
- Move role out of JWT and into `control_subject_roles`.
- Keep commerce data reachable through the existing numeric user ID until downstream services are migrated.

Backfill rules:

- Each existing `users` row gets one `auth_subjects` row and one `control_user_profiles` row.
- `users.password_hash` moves to `auth_password_credentials`.
- `users.totp_*` moves to `auth_mfa_totp_factors`.
- `users.role` becomes rows in `control_subject_roles`.
- `users.status` maps to subject status.
- `users.id` is preserved as `legacy_user_id` for transition.

### 5.4 Postgres schema note

The target architecture says to use separate Postgres schemas for auth- versus control-owned tables.

Planned physical target:

- auth schema: `auth_*` tables
- control schema: `control_*` tables

Open implementation check:

- Validate early whether the current Ent setup and migration tooling can comfortably target non-public Postgres schemas.
- If Ent support is awkward, use prefixed tables in `public` only as a temporary implementation fallback and record that explicitly as a deviation. Do not silently drift from the target design.

## 6. Session, Token, and Security Model

### 6.1 Access tokens

- Algorithm: `ES256`
- TTL: 5 minutes
- Claims are minimal and fixed:
  - `iss`
  - `aud`
  - `sub`
  - `sid`
  - `av`
  - `amr`
  - `iat`
  - `exp`
- `sub` is immutable `subject_id` UUID.
- `sid` is the session UUID.
- `av` is `subject_auth_version`.
- `amr` is a compact auth method string such as `pwd`, `oidc`, or `pwd+totp`.
- Roles are not embedded in JWT claims.

### 6.2 Refresh tokens

- Opaque random values only.
- Never stored in plaintext.
- Persist only token hashes.
- Rotate on every refresh.
- 7-day idle timeout.
- 30-day absolute lifetime.
- Reuse detection revokes the whole session.

Recommended implementation detail:

- Keep the current refresh token hash in `auth_sessions.current_refresh_token_hash`.
- Keep token lineage in `auth_refresh_tokens` so reuse of a rotated token can be detected and treated as compromise.

### 6.3 Sessions

Each authenticated browser session gets a durable `auth_sessions` row containing:

- `sid`
- `subject_id`
- `status`
- `amr`
- `last_seen_at`
- `expires_at`
- `revoked_at`
- current refresh token hash

`subject_auth_version` is stored on `auth_subjects` and bumped on:

- password change
- logout-all
- MFA reset
- account disable
- any security-sensitive auth reset

### 6.4 Session verification flow

Control auth middleware should:

1. Read the access JWT from the cookie, not the `Authorization` header for normal browser traffic.
2. Verify the ES256 JWT locally against the current key set and expected `iss` and `aud`.
3. Read a cached session snapshot from Redis keyed by `sid`.
4. On Redis miss, load the session and subject from Postgres and repopulate Redis.
5. Reject the request if:
   - session is revoked
   - subject is disabled
   - `av` does not match current subject `auth_version`
6. Resolve roles through `access` and attach the auth subject to Gin context.

This keeps revocation immediate even though access tokens are short-lived.

### 6.5 Cookies

Browser auth should use:

- access cookie
  - `HttpOnly`
  - `Secure`
  - `SameSite=Lax`
- refresh cookie
  - `HttpOnly`
  - `Secure`
  - `SameSite=Lax`
- CSRF cookie
  - `Secure`
  - `SameSite=Lax`
  - readable by JS so the SPA can mirror it in `X-CSRF-Token`
- pending-registration cookie
  - opaque
  - `HttpOnly`
  - `Secure`
  - `SameSite=Lax`
- auth-flow cookie
  - opaque
  - `HttpOnly`
  - `Secure`
  - `SameSite=Lax`

The SPA should never receive access or refresh tokens in JSON or in redirect URLs for browser login flows.

### 6.6 CSRF

- All mutating control-plane routes should require double-submit CSRF.
- `GET /api/v1/bootstrap` should always mint or refresh the CSRF cookie so anonymous pages can call login, registration, and forgot-password safely.
- The frontend must mirror the CSRF cookie into `X-CSRF-Token`.

### 6.7 JWKS and key management

- Expose a JWKS endpoint from `control`.
- Prefer `/.well-known/jwks.json` on the control server.
- Phase 1 default: config-backed ES256 key ring from mounted secrets.
- Keep the active signing key private key plus any still-valid public verification keys for rotation overlap.

This avoids introducing a DB-managed signing-key lifecycle in the same refactor unless operations require it.

## 7. API Redesign

### 7.1 Canonical browser BFF routes

The new user-facing surface is:

- `GET /api/v1/bootstrap`
- `POST /api/v1/session/login`
- `POST /api/v1/session/login/totp`
- `DELETE /api/v1/session`
- `DELETE /api/v1/sessions`
- `POST /api/v1/session/refresh`
- `POST /api/v1/registration/preflight`
- `POST /api/v1/registration/email-code`
- `POST /api/v1/registration`
- `POST /api/v1/registration/complete`
- `POST /api/v1/password/forgot`
- `POST /api/v1/password/reset`
- `POST /api/v1/me/password/change`
- `PATCH /api/v1/me`
- `GET /api/v1/me/mfa/totp`
- `POST /api/v1/me/mfa/totp/send-code`
- `POST /api/v1/me/mfa/totp/setup`
- `POST /api/v1/me/mfa/totp/enable`
- `DELETE /api/v1/me/mfa/totp`
- `GET /api/v1/oauth/{provider}/start`
- `GET /api/v1/oauth/{provider}/callback`

### 7.2 Old-to-new route mapping

- `/api/v1/settings/public` and `/api/v1/auth/me`
  - replaced by `GET /api/v1/bootstrap`
- `/api/v1/auth/login`
  - replaced by `POST /api/v1/session/login`
- `/api/v1/auth/login/2fa`
  - replaced by `POST /api/v1/session/login/totp`
- `/api/v1/auth/logout`
  - replaced by `DELETE /api/v1/session`
- `/api/v1/auth/revoke-all-sessions`
  - replaced by `DELETE /api/v1/sessions`
- `/api/v1/auth/refresh`
  - replaced by `POST /api/v1/session/refresh`
- `/api/v1/auth/send-verify-code`
  - replaced by `POST /api/v1/registration/email-code`
- `/api/v1/auth/register`
  - replaced by `POST /api/v1/registration`
- `/api/v1/auth/validate-promo-code`
  - folded into `POST /api/v1/registration/preflight`
- `/api/v1/auth/validate-invitation-code`
  - folded into `POST /api/v1/registration/preflight`
- `/api/v1/auth/forgot-password`
  - replaced by `POST /api/v1/password/forgot`
- `/api/v1/auth/reset-password`
  - replaced by `POST /api/v1/password/reset`
- `/api/v1/user/profile`
  - replaced by `PATCH /api/v1/me` and the `profile` fragment in `bootstrap`
- `/api/v1/user/password`
  - replaced by `POST /api/v1/me/password/change`
- `/api/v1/user/totp/*`
  - replaced by `/api/v1/me/mfa/totp*`
- `/api/v1/auth/oauth/*`
  - replaced by `/api/v1/oauth/{provider}/*`

These legacy routes can exist briefly as implementation shims inside the branch if that helps land the frontend and backend incrementally, but the merged result should remove them.

### 7.3 Bootstrap payload

`GET /api/v1/bootstrap` becomes the canonical SPA boot endpoint.

Suggested response shape:

```json
{
  "public": {
    "site_name": "Sub2API",
    "run_mode": "standard",
    "registration_enabled": true,
    "email_verification_required": true,
    "invitation_required": false,
    "turnstile_site_key": "...",
    "oauth": {
      "oidc_enabled": true,
      "linuxdo_enabled": false
    }
  },
  "csrf_token": "opaque-csrf-token",
  "auth": {
    "authenticated": true
  },
  "subject": {
    "id": "uuid",
    "status": "active"
  },
  "profile": {
    "username": "alice",
    "notes": ""
  },
  "roles": ["admin"],
  "mfa": {
    "totp_enabled": true,
    "verification_method": "email"
  },
  "session": {
    "sid": "uuid",
    "amr": "pwd+totp",
    "last_seen_at": "2026-04-10T00:00:00Z",
    "expires_at": "2026-05-10T00:00:00Z"
  }
}
```

Important behavior:

- Always returns `200`.
- Always includes public shell settings and a CSRF token.
- Includes auth/me fragments only when authenticated.
- Replaces separate boot-time fetches for public settings and current user.

### 7.4 Login and TOTP flow

- `POST /api/v1/session/login`
  - accepts email, password, Turnstile token
  - on normal success, sets cookies and returns bootstrap payload
  - if TOTP is required, does not create a full authenticated session
  - returns `mfa_required` plus an opaque `login_challenge_id`
- `POST /api/v1/session/login/totp`
  - consumes `login_challenge_id` plus TOTP code
  - on success, sets cookies and returns bootstrap payload

Implementation note:

- Current Redis-backed temporary TOTP login session logic in `backend/internal/repository/totp_cache.go` can be adapted into `identity` flow storage if desired, but the external API and naming should change from `temp_token` to `login_challenge_id`.

### 7.5 Registration flow

- `POST /api/v1/registration/preflight`
  - single policy/validation endpoint replacing invite and promo probes
- `POST /api/v1/registration/email-code`
  - registration email verification only
- `POST /api/v1/registration`
  - performs one DB transaction for subject, profile, first entitlements, and first session
- `POST /api/v1/registration/complete`
  - invitation-completion endpoint for pending OAuth registration

Implementation note:

- Registration must remain transactional in Postgres. Do not introduce a saga or outbox for this flow.

### 7.6 Password flow

- `POST /api/v1/password/forgot`
  - request reset email
- `POST /api/v1/password/reset`
  - completes reset and revokes all sessions
- `POST /api/v1/me/password/change`
  - authenticated change-password endpoint
  - rotates current session and revokes all other sessions

Password change should move out of `UserService` and into `identity`.

### 7.7 OAuth flow

Browser OAuth must stop using fragments for token delivery.

Target behavior:

- `GET /api/v1/oauth/{provider}/start`
  - creates `auth_flows` record and opaque cookie-backed flow state
- `GET /api/v1/oauth/{provider}/callback`
  - validates server-side flow state
  - matches federated identity only by `provider + issuer + subject`
  - if registration is complete:
    - create session
    - set cookies
    - redirect to SPA shell
  - if invitation is still required:
    - create `auth_registration_challenges` entry
    - set pending-registration cookie
    - redirect to registration-complete page

Specific repo implication:

- The current LinuxDo synthetic email and OIDC identity-key email hacks can be retired once `auth_federated_identities` becomes the real source of identity linkage.
- No access or refresh token should appear in URL query strings or fragments.

## 8. Middleware and Authorization Changes

### 8.1 Replace current JWT middleware

Current `backend/internal/server/middleware/jwt_auth.go` is tied to:

- bearer header auth
- numeric user IDs
- `TokenVersion`

Replace it with middleware that:

- reads the access cookie
- validates ES256 JWT
- loads session and subject snapshot
- attaches `subject_id`, `sid`, `amr`, and resolved roles to Gin context
- optionally also attaches `legacy_user_id` during the transition period

### 8.2 Replace current admin auth middleware

Current `backend/internal/server/middleware/admin_auth.go` allows:

- admin API key
- bearer JWT
- websocket subprotocol JWT

Target behavior:

- remove admin API key support
- use the same authenticated session model for admin and user routes
- enforce admin access through `access` role checks

Open item:

- Admin websocket endpoints currently rely on JWTs in `Sec-WebSocket-Protocol`.
- If those endpoints still matter, move them to cookie-backed auth plus role validation and origin checks.
- Do not keep the admin API key path alive just for websocket compatibility.

### 8.3 Backend mode guard

`backend/internal/server/middleware/backend_mode_guard.go` currently whitelists only the legacy auth route names.

It must be updated to the new route set:

- allow login, TOTP completion, logout, refresh, and bootstrap for admin web auth
- block registration and self-service flows when backend mode is enabled for non-admins

## 9. Frontend Refactor Plan

### 9.1 API client

Replace the current token-in-localStorage model in `frontend/src/api/client.ts` with:

- `withCredentials: true`
- no `Authorization` header injection for normal browser requests
- CSRF header injection from the CSRF cookie
- one retry path on `401` that calls `POST /api/v1/session/refresh` and retries the original request after the browser updates cookies

### 9.2 Auth store

Replace `frontend/src/stores/auth.ts` responsibilities:

- remove localStorage-backed token persistence
- remove proactive token-expiry scheduling based on `expires_in`
- derive auth state from `bootstrap`
- store current user/profile/roles/session summary in memory
- expose a `loadBootstrap()` or equivalent instead of `checkAuth()`

The router and app startup should stop assuming that JWT presence in localStorage equals auth.

### 9.3 New frontend API modules

Suggested split:

```text
frontend/src/api/
  bootstrap.ts
  session.ts
  registration.ts
  password.ts
  me.ts
  oauth.ts
```

Retire or sharply reduce:

- `frontend/src/api/auth.ts`
- `frontend/src/api/totp.ts`

### 9.4 SPA boot flow

Current boot behavior is fragmented:

- public settings are fetched by many views
- auth state is restored from localStorage
- `/auth/me` refreshes the user later

Target boot behavior:

1. Load `GET /api/v1/bootstrap` early.
2. Initialize app public settings from the `public` slice.
3. Initialize auth/user/admin state from the same payload.
4. Use the same endpoint to recover from refreshes and post-login state changes.

### 9.5 OAuth callback views

Current callback pages parse fragments and persist tokens.

Target callback behavior:

- Browser lands on a frontend route after the server callback already set cookies.
- The page only needs to:
  - show a progress state
  - call `bootstrap`
  - handle pending-registration mode if the server redirected there
- No token parsing.
- No token storage.

### 9.6 Admin settings UI cleanup

Remove:

- admin API key settings API calls in `frontend/src/api/admin/settings.ts`
- corresponding UI
- associated i18n strings documenting `x-api-key` usage

## 10. Implementation Phases

### Phase 0: Pre-flight decisions

- [ ] Confirm JWT `iss` and `aud` values for control.
- [ ] Confirm cookie names and cookie path/domain strategy behind the current control + frontend deployment shape.
- [ ] Confirm whether the phase uses true Postgres schemas or prefixed tables in `public`.
- [ ] Confirm whether ES256 key rotation is config-backed only in this phase.
- [ ] Confirm how admin websocket auth should work after header-token removal.

Exit criteria:

- No unresolved infrastructure decision blocks the data model or middleware work.

### Phase 1: Schema and repository foundation

- [ ] Add Ent schema definitions for the new auth/control tables.
- [ ] Generate Ent code and migrations.
- [ ] Add backfill migration from `users` into `auth_subjects`, `auth_password_credentials`, `auth_mfa_totp_factors`, `control_user_profiles`, and `control_subject_roles`.
- [ ] Add a resolver abstraction for `subject_id <-> legacy_user_id`.
- [ ] Add Redis cache shapes for:
  - session snapshot
  - subject status/auth version
  - resolved roles

Exit criteria:

- A subject and session can be created and reloaded from Postgres and Redis without touching the old auth routes yet.

### Phase 2: Identity and access services

- [ ] Introduce `controlplane/identity` services for:
  - JWT signing/verification
  - refresh rotation
  - session creation/revocation
  - password reset/change
  - TOTP management
- [ ] Introduce `controlplane/access` role lookup and middleware.
- [ ] Introduce cookie helpers and CSRF middleware.
- [ ] Expose JWKS endpoint.
- [ ] Replace `TokenVersion` logic with `auth_version`.

Exit criteria:

- New cookie-backed sessions work internally.
- Revocation and auth-version mismatch are enforced via middleware.

### Phase 3: Registration and OAuth carve-out

- [ ] Introduce `controlplane/registration`.
- [ ] Move registration policy evaluation out of `AuthService`.
- [ ] Consolidate promo and invitation validation into registration preflight.
- [ ] Replace pending OAuth token fragments with server-side `auth_flows` and `auth_registration_challenges`.
- [ ] Keep admin/provider-account OAuth flows untouched in `provideraccounts`.

Exit criteria:

- Registration and browser OAuth can complete end-to-end without URL tokens.

### Phase 4: HTTP surface replacement

- [ ] Replace route registration in `backend/internal/server/router.go` and `backend/internal/server/routes/*.go` with the new BFF route groups.
- [ ] Add handlers for:
  - bootstrap
  - session
  - registration
  - password
  - me
  - oauth
- [ ] Remove `/auth/*`, `/user/profile`, `/user/password`, `/user/totp/*`, `/settings/public`, and `/auth/me` from the merged result.
- [ ] Update backend mode guard to the new route set.
- [ ] Remove admin API key validation from admin auth middleware and settings routes.

Exit criteria:

- The new HTTP contract is the only user-facing auth/profile/settings surface left.

### Phase 5: Frontend migration

- [ ] Replace `auth.ts` and `totp.ts` client usage with the new API modules.
- [ ] Update `apiClient` for cookies and CSRF.
- [ ] Rewrite the auth store around `bootstrap`.
- [ ] Update router guards to use subject/role state from bootstrap.
- [ ] Update login, register, forgot/reset password, profile, and TOTP views to the new endpoints.
- [ ] Simplify OAuth callback views to no-token pages.
- [ ] Remove admin API key UI and docs.

Exit criteria:

- The SPA works entirely with cookie-backed sessions and bootstrap responses.

### Phase 6: Cleanup and verification

- [ ] Delete now-dead legacy handlers, route registrations, DTOs, and token helpers.
- [ ] Delete localStorage auth helpers.
- [ ] Delete fragment-token OAuth behavior.
- [ ] Update docs and API contract tests.
- [ ] Run full verification.

Exit criteria:

- No production code path relies on header bearer tokens or admin API keys for control-plane browser/admin auth.

## 11. Test Plan

The requested test plan is correct; map it onto the existing repo structure as follows.

### Backend unit tests

- JWT signing and JWKS verification
  - new tests under `backend/internal/controlplane/identity/*_test.go`
- auth middleware cases
  - replace or rewrite current `backend/internal/server/middleware/jwt_auth_test.go`
  - cover:
    - valid access token
    - expired access token
    - revoked session
    - disabled subject
    - `auth_version` mismatch
    - malformed cookie
    - Redis miss with Postgres fallback
- admin access middleware cases
  - replace or rewrite current `backend/internal/server/middleware/admin_auth_test.go`
  - verify role checks, not JWT-embedded role checks
- refresh token rotation
  - normal refresh
  - reused refresh token
  - expired refresh token
  - revoked session
  - logout-all invalidation
- login with TOTP
  - password login without TOTP
  - password login returning `mfa_required`
  - valid TOTP completion
  - invalid/expired login challenge
  - invalid TOTP code
- registration preflight
  - registration disabled
  - blocked email suffix
  - invitation required/valid/used/invalid
  - promo valid/expired/disabled/overused

### Backend integration tests

- transactional registration rollback on:
  - invitation redemption failure
  - promo validation failure
  - profile creation failure
  - default subscription creation failure
- password reset revokes prior sessions immediately
- password change revokes other sessions and rotates current one
- logout current session versus logout-all semantics
- admin role removal blocks the next admin request immediately
- OAuth callback never leaks tokens in redirects
- `bootstrap` works for authenticated and unauthenticated states
- gateway behavior remains unchanged
- provider-account OAuth onboarding and worker token refresh remain unaffected

Suggested existing files to update or replace:

- `backend/internal/integration/e2e_user_flow_test.go`
- `backend/internal/server/api_contract_test.go`
- `backend/internal/server/router_role_test.go`
- `backend/internal/server/routes/auth_rate_limit_test.go`
- `backend/internal/server/middleware/backend_mode_guard_test.go`

### Frontend tests

- auth store bootstrap initialization
- `401 -> refresh -> retry` behavior with cookies
- route guards for authenticated and admin states
- login and TOTP flows
- registration preflight and complete registration flows
- callback pages no longer parsing URL tokens

Suggested existing files to update or replace:

- `frontend/src/stores/__tests__/auth.spec.ts`
- `frontend/src/router/__tests__/guards.spec.ts`
- `frontend/src/api/__tests__/*`

## 12. Risks and Open Questions

- Ent and migration support for separate Postgres schemas needs to be validated early.
- The `users.id` to `subject_id` transition is the largest structural risk. Do not attempt a wholesale commerce re-key unless this branch is explicitly widened.
- Admin websocket auth needs a concrete cookie-based replacement if those endpoints are still in use.
- Cookie domain/path behavior must be tested against the deployed control + frontend sidecar topology.
- ES256 key sourcing and rotation need an operational decision before implementation starts.
- Existing backend-mode semantics may need small product clarification once `bootstrap` is always public but self-service mutations are blocked.

## 13. Recommended Execution Order

1. Resolve the pre-flight decisions in Phase 0.
2. Land the new tables and subject mapping first.
3. Land JWT/JWKS/session/access infrastructure next.
4. Land new handlers and routes in parallel with frontend migration.
5. Remove legacy routes and admin API key support only after the new SPA flow is green in the same branch.
6. Run backend unit tests, backend integration tests, and frontend test suites before merge.
