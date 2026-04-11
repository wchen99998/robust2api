/**
 * Authentication API endpoints (cookie/session based)
 * Uses the control BFF surface centered around bootstrap/session/registration/password/me/oauth.
 */

import { apiClient } from './client'
import type {
  LoginRequest,
  RegisterRequest,
  CurrentUserResponse,
  SendVerifyCodeRequest,
  SendVerifyCodeResponse,
  PublicSettings,
  TotpLoginResponse,
  TotpLogin2FARequest,
  BootstrapResponse,
  BootstrapPendingRegistration,
  BootstrapSession,
  BootstrapAuthCapabilities,
  BootstrapAuthProvider,
  RegistrationPreflightRequest,
  RegistrationPreflightResponse
} from '@/types'

export type LoginResponse = BootstrapResponse | TotpLoginResponse

function isObjectRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function normalizePublicSettings(raw: unknown): PublicSettings {
  const payload = isObjectRecord(raw) ? raw : {}
  return {
    registration_enabled: Boolean(payload.registration_enabled),
    email_verify_enabled: Boolean(
      payload.email_verify_enabled ?? payload.email_verification_required
    ),
    registration_email_suffix_whitelist: Array.isArray(payload.registration_email_suffix_whitelist)
      ? (payload.registration_email_suffix_whitelist as string[])
      : [],
    promo_code_enabled: Boolean(payload.promo_code_enabled),
    password_reset_enabled: Boolean(payload.password_reset_enabled),
    invitation_code_enabled: Boolean(payload.invitation_code_enabled),
    turnstile_enabled: Boolean(payload.turnstile_enabled),
    turnstile_site_key: String(payload.turnstile_site_key || ''),
    site_name: String(payload.site_name || ''),
    site_logo: String(payload.site_logo || ''),
    site_subtitle: String(payload.site_subtitle || ''),
    api_base_url: String(payload.api_base_url || ''),
    grafana_url: String(payload.grafana_url || ''),
    contact_info: String(payload.contact_info || ''),
    doc_url: String(payload.doc_url || ''),
    home_content: String(payload.home_content || ''),
    hide_ccs_import_button: Boolean(payload.hide_ccs_import_button),
    purchase_subscription_enabled: Boolean(payload.purchase_subscription_enabled),
    purchase_subscription_url: String(payload.purchase_subscription_url || ''),
    custom_menu_items: Array.isArray(payload.custom_menu_items) ? payload.custom_menu_items : [],
    custom_endpoints: Array.isArray(payload.custom_endpoints) ? payload.custom_endpoints : [],
    linuxdo_oauth_enabled: Boolean(payload.linuxdo_oauth_enabled),
    oidc_oauth_enabled: Boolean(payload.oidc_oauth_enabled),
    oidc_oauth_provider_name: String(payload.oidc_oauth_provider_name || ''),
    backend_mode_enabled: Boolean(payload.backend_mode_enabled),
    version: String(payload.version || '')
  }
}

export function normalizeBootstrapResponse(raw: unknown): BootstrapResponse {
  const payload = isObjectRecord(raw) ? raw : {}
  const authStateRaw = isObjectRecord(payload.auth_state)
    ? payload.auth_state
    : isObjectRecord(payload.session)
      ? payload.session
      : {}
  const meRaw = isObjectRecord(payload.me)
    ? payload.me
    : isObjectRecord(payload.identity)
      ? payload.identity
      : {}
  const subjectRaw = isObjectRecord(payload.subject)
    ? payload.subject
    : isObjectRecord(meRaw.subject)
      ? meRaw.subject
      : {}
  const profileRaw = isObjectRecord(payload.profile)
    ? payload.profile
    : isObjectRecord(meRaw.profile)
      ? meRaw.profile
      : isObjectRecord(meRaw.user)
        ? meRaw.user
        : null
  const sessionRaw = isObjectRecord(payload.session)
    ? payload.session
    : isObjectRecord(meRaw.session)
      ? meRaw.session
      : null
  const mfaRaw = isObjectRecord(payload.mfa)
    ? payload.mfa
    : isObjectRecord(meRaw.mfa)
      ? meRaw.mfa
      : {}

  const publicSettingsRaw =
    payload.public_settings ??
    payload.settings ??
    payload.public ??
    payload.config ??
    {}

  const roleList = Array.isArray(payload.roles)
    ? (payload.roles as string[])
    : Array.isArray(meRaw.roles)
      ? (meRaw.roles as string[])
      : []

  const explicitRunMode =
    payload.run_mode === 'simple' || payload.run_mode === 'standard'
      ? payload.run_mode
      : undefined

  const hasProfileLikeShape =
    isObjectRecord(profileRaw) &&
    (typeof profileRaw.id === 'number' ||
      typeof profileRaw.email === 'string' ||
      typeof profileRaw.username === 'string')

  const meUser = hasProfileLikeShape
    ? ({
        ...(profileRaw as unknown as CurrentUserResponse),
        role:
          typeof profileRaw.role === 'string'
            ? profileRaw.role
            : roleList.includes('admin')
              ? 'admin'
              : 'user',
        run_mode:
          (profileRaw.run_mode as 'standard' | 'simple' | undefined) ??
          explicitRunMode ??
          undefined
      } as CurrentUserResponse)
    : undefined

  const hasSessionLikeShape =
    isObjectRecord(sessionRaw) &&
    (typeof sessionRaw.session_id === 'string' ||
      typeof sessionRaw.sessionID === 'string' ||
      typeof sessionRaw.expires_at === 'string' ||
      typeof sessionRaw.expiresAt === 'string')

  const sessionNormalized: BootstrapSession | null = hasSessionLikeShape
    ? {
        session_id:
          typeof sessionRaw.session_id === 'string'
            ? sessionRaw.session_id
            : typeof sessionRaw.sessionID === 'string'
              ? sessionRaw.sessionID
              : '',
        expires_at: String(sessionRaw.expires_at ?? sessionRaw.expiresAt ?? ''),
        absolute_expires_at: String(
          sessionRaw.absolute_expires_at ?? sessionRaw.absoluteExpiresAt ?? ''
        ),
        last_seen_at: String(sessionRaw.last_seen_at ?? sessionRaw.lastSeenAt ?? '')
      }
    : null

  const pendingRegistrationRaw = isObjectRecord(payload.pending_registration)
    ? payload.pending_registration
    : null
  const pendingRegistration: BootstrapPendingRegistration | null = isObjectRecord(
    pendingRegistrationRaw
  )
    ? ({
        challenge_id: String(pendingRegistrationRaw.challenge_id ?? ''),
        provider: String(pendingRegistrationRaw.provider ?? ''),
        email: String(pendingRegistrationRaw.email ?? ''),
        registration_email:
          typeof pendingRegistrationRaw.registration_email === 'string'
            ? pendingRegistrationRaw.registration_email
            : undefined,
        username:
          typeof pendingRegistrationRaw.username === 'string'
            ? pendingRegistrationRaw.username
            : undefined,
        redirect_to:
          typeof pendingRegistrationRaw.redirect_to === 'string'
            ? pendingRegistrationRaw.redirect_to
            : undefined,
        expires_at: String(pendingRegistrationRaw.expires_at ?? '')
      } as BootstrapPendingRegistration)
    : null
  const authCapabilitiesRaw = isObjectRecord(payload.auth_capabilities)
    ? payload.auth_capabilities
    : null
  const providerHint =
    typeof authCapabilitiesRaw?.provider === 'string' ? authCapabilitiesRaw.provider : 'local'
  const authCapabilities: BootstrapAuthCapabilities | undefined = authCapabilitiesRaw
    ? {
        provider: providerHint,
        password_login_enabled:
          typeof authCapabilitiesRaw.password_login_enabled === 'boolean'
            ? authCapabilitiesRaw.password_login_enabled
            : true,
        registration_enabled:
          typeof authCapabilitiesRaw.registration_enabled === 'boolean'
            ? authCapabilitiesRaw.registration_enabled
            : Boolean((publicSettingsRaw as Record<string, unknown>).registration_enabled),
        email_verification_enabled:
          typeof authCapabilitiesRaw.email_verification_enabled === 'boolean'
            ? authCapabilitiesRaw.email_verification_enabled
            : Boolean((publicSettingsRaw as Record<string, unknown>).email_verify_enabled),
        password_reset_enabled:
          typeof authCapabilitiesRaw.password_reset_enabled === 'boolean'
            ? authCapabilitiesRaw.password_reset_enabled
            : Boolean((publicSettingsRaw as Record<string, unknown>).password_reset_enabled),
        password_change_enabled:
          typeof authCapabilitiesRaw.password_change_enabled === 'boolean'
            ? authCapabilitiesRaw.password_change_enabled
            : true,
        mfa_self_service_enabled:
          typeof authCapabilitiesRaw.mfa_self_service_enabled === 'boolean'
            ? authCapabilitiesRaw.mfa_self_service_enabled
            : true,
        profile_self_service_enabled:
          typeof authCapabilitiesRaw.profile_self_service_enabled === 'boolean'
            ? authCapabilitiesRaw.profile_self_service_enabled
            : true
      }
    : undefined
  const providersRaw = Array.isArray(payload.auth_providers) ? payload.auth_providers : []
  const authProviders: BootstrapAuthProvider[] = providersRaw
    .map((item) => {
      if (!isObjectRecord(item)) return null
      const id = typeof item.id === 'string' ? item.id.trim() : ''
      const startPath = typeof item.start_path === 'string' ? item.start_path.trim() : ''
      if (!id || !startPath) return null
      return {
        id,
        type: typeof item.type === 'string' ? item.type : 'oauth',
        display_name:
          typeof item.display_name === 'string' && item.display_name.trim()
            ? item.display_name
            : id,
        start_path: startPath
      }
    })
    .filter((item): item is BootstrapAuthProvider => item !== null)

  if (authProviders.length === 0) {
    if ((publicSettingsRaw as Record<string, unknown>).linuxdo_oauth_enabled) {
      authProviders.push({
        id: 'linuxdo',
        type: 'oauth',
        display_name: 'Linux.do',
        start_path: '/api/v1/oauth/linuxdo/start'
      })
    }
    if ((publicSettingsRaw as Record<string, unknown>).oidc_oauth_enabled) {
      const oidcName =
        typeof (publicSettingsRaw as Record<string, unknown>).oidc_oauth_provider_name === 'string'
          ? String((publicSettingsRaw as Record<string, unknown>).oidc_oauth_provider_name).trim()
          : ''
      authProviders.push({
        id: 'oidc',
        type: 'oidc',
        display_name: oidcName || 'OIDC',
        start_path: '/api/v1/oauth/oidc/start'
      })
    }
  }

  return {
    csrf_token: typeof payload.csrf_token === 'string' ? payload.csrf_token : undefined,
    run_mode: explicitRunMode,
    public_settings: normalizePublicSettings(publicSettingsRaw),
    auth_capabilities: authCapabilities,
    auth_providers: authProviders,
    auth_state: {
      authenticated: Boolean(
        payload.authenticated ??
          authStateRaw.authenticated ??
          authStateRaw.is_authenticated ??
          (meUser && meUser.id !== undefined)
      ),
      mfa_required: Boolean(authStateRaw.mfa_required),
      login_challenge_id:
        typeof authStateRaw.login_challenge_id === 'string'
          ? authStateRaw.login_challenge_id
          : undefined,
      refresh_available:
        typeof payload.refresh_available === 'boolean' ? payload.refresh_available : undefined
    },
    me: {
      subject_id:
        typeof subjectRaw.subject_id === 'string'
          ? subjectRaw.subject_id
          : typeof meRaw.subject_id === 'string'
            ? meRaw.subject_id
            : typeof meRaw.subjectID === 'string'
              ? meRaw.subjectID
              : undefined,
      sid:
        typeof subjectRaw.session_id === 'string'
          ? subjectRaw.session_id
          : typeof meRaw.sid === 'string'
            ? meRaw.sid
            : sessionNormalized?.session_id || undefined,
      roles: roleList.length > 0 ? roleList : undefined,
      primary_role:
        typeof payload.primary_role === 'string'
          ? payload.primary_role
          : typeof meRaw.primary_role === 'string'
            ? meRaw.primary_role
            : undefined,
      user: meUser ?? null,
      profile: hasProfileLikeShape ? (profileRaw as Partial<CurrentUserResponse>) : null,
      mfa: isObjectRecord(mfaRaw) ? (mfaRaw as any) : null,
      session: sessionNormalized
    },
    pending_registration: pendingRegistration
  }
}

function isTotpChallengeResponse(data: unknown): data is TotpLoginResponse {
  if (!isObjectRecord(data)) return false
  return Boolean(data.mfa_required || data.requires_2fa || data.login_challenge_id || data.temp_token)
}

export function isTotp2FARequired(response: LoginResponse): response is TotpLoginResponse {
  return isTotpChallengeResponse(response) && Boolean(response.mfa_required ?? response.requires_2fa)
}

export async function bootstrap(): Promise<BootstrapResponse> {
  const { data } = await apiClient.get<unknown>('/bootstrap')
  return normalizeBootstrapResponse(data)
}

export async function getPublicSettings(): Promise<PublicSettings> {
  const boot = await bootstrap()
  return boot.public_settings
}

export async function login(credentials: LoginRequest): Promise<LoginResponse> {
  const { data } = await apiClient.post<unknown>('/session/login', credentials)
  if (isTotpChallengeResponse(data)) {
    return {
      requires_2fa: Boolean(data.requires_2fa ?? data.mfa_required),
      mfa_required: Boolean(data.mfa_required ?? data.requires_2fa),
      temp_token: typeof data.temp_token === 'string' ? data.temp_token : undefined,
      login_challenge_id:
        typeof data.login_challenge_id === 'string' ? data.login_challenge_id : undefined,
      user_email_masked:
        typeof data.user_email_masked === 'string'
          ? data.user_email_masked
          : typeof data.masked_email === 'string'
            ? data.masked_email
            : undefined,
      masked_email: typeof data.masked_email === 'string' ? data.masked_email : undefined
    }
  }
  return normalizeBootstrapResponse(data)
}

export async function login2FA(request: TotpLogin2FARequest): Promise<BootstrapResponse> {
  const challengeID = request.login_challenge_id || request.temp_token
  const { data } = await apiClient.post<unknown>('/session/login/totp', {
    login_challenge_id: challengeID,
    totp_code: request.totp_code
  })
  return normalizeBootstrapResponse(data)
}

export async function logout(): Promise<void> {
  await apiClient.delete('/session')
}

export async function revokeAllSessions(): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>('/sessions')
  return data
}

export async function refreshSession(): Promise<BootstrapResponse> {
  const { data } = await apiClient.post<unknown>('/session/refresh', {})
  return normalizeBootstrapResponse(data)
}

export async function register(userData: RegisterRequest): Promise<BootstrapResponse> {
  const { data } = await apiClient.post<unknown>('/registration', {
    email: userData.email,
    password: userData.password,
    verification_code: userData.verification_code ?? userData.verify_code,
    turnstile_token: userData.turnstile_token,
    promo_code: userData.promo_code,
    invitation_code: userData.invitation_code
  })
  return normalizeBootstrapResponse(data)
}

export async function registrationPreflight(
  payload: RegistrationPreflightRequest
): Promise<RegistrationPreflightResponse> {
  const { data } = await apiClient.post<RegistrationPreflightResponse>('/registration/preflight', payload)
  return data
}

export async function sendVerifyCode(
  request: SendVerifyCodeRequest
): Promise<SendVerifyCodeResponse> {
  const { data } = await apiClient.post<SendVerifyCodeResponse>('/registration/email-code', request)
  return data
}

export interface ValidatePromoCodeResponse {
  valid: boolean
  bonus_amount?: number
  error_code?: string
  message?: string
}

export async function validatePromoCode(code: string): Promise<ValidatePromoCodeResponse> {
  const result = await registrationPreflight({ promo_code: code })
  const valid = ['valid', 'ok', 'accepted'].includes(String(result.promo_status || '').toLowerCase())
  return {
    valid,
    bonus_amount: undefined,
    error_code: valid ? undefined : (result.promo_status || result.errors?.[0] || 'INVALID_PROMO'),
    message: result.errors?.[0]
  }
}

export interface ValidateInvitationCodeResponse {
  valid: boolean
  error_code?: string
}

export async function validateInvitationCode(code: string): Promise<ValidateInvitationCodeResponse> {
  const result = await registrationPreflight({ invitation_code: code })
  const valid = ['valid', 'ok', 'accepted'].includes(
    String(result.invitation_status || '').toLowerCase()
  )
  return {
    valid,
    error_code: valid
      ? undefined
      : (result.invitation_status || result.errors?.[0] || 'INVALID_INVITATION')
  }
}

export interface ForgotPasswordRequest {
  email: string
  turnstile_token?: string
}

export interface ForgotPasswordResponse {
  message: string
}

export async function forgotPassword(request: ForgotPasswordRequest): Promise<ForgotPasswordResponse> {
  const { data } = await apiClient.post<ForgotPasswordResponse>('/password/forgot', request)
  return data
}

export interface ResetPasswordRequest {
  email?: string
  token?: string
  code?: string
  new_password: string
}

export interface ResetPasswordResponse {
  message: string
}

export async function resetPassword(request: ResetPasswordRequest): Promise<ResetPasswordResponse> {
  const payload = {
    ...request,
    token: request.token ?? request.code
  }
  const { data } = await apiClient.post<ResetPasswordResponse>('/password/reset', payload)
  return data
}

export async function completeOAuthRegistration(invitationCode: string): Promise<BootstrapResponse> {
  const { data } = await apiClient.post<unknown>('/registration/complete', {
    invitation_code: invitationCode
  })
  return normalizeBootstrapResponse(data)
}

export async function getCurrentUser(): Promise<{ data: CurrentUserResponse }> {
  const boot = await bootstrap()
  if (!boot.me?.user) {
    throw new Error('Not authenticated')
  }
  return { data: boot.me.user as CurrentUserResponse }
}

export function isAuthenticated(): boolean {
  return !!localStorage.getItem('auth_user')
}

export const authAPI = {
  bootstrap,
  login,
  login2FA,
  isTotp2FARequired,
  register,
  getCurrentUser,
  logout,
  isAuthenticated,
  getPublicSettings,
  sendVerifyCode,
  validatePromoCode,
  validateInvitationCode,
  forgotPassword,
  resetPassword,
  refreshSession,
  revokeAllSessions,
  registrationPreflight,
  completeOAuthRegistration
}

export default authAPI
