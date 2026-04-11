package routes

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/middleware"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterAuthRoutes 注册认证相关路由
func RegisterAuthRoutes(
	v1 *gin.RouterGroup,
	h *handler.ControlHandlers,
	jwtAuth servermiddleware.JWTAuthMiddleware,
	redisClient *redis.Client,
	settingService *service.SettingService,
) {
	// 创建速率限制器
	rateLimiter := middleware.NewRateLimiter(redisClient)

	// 公开接口
	auth := v1.Group("/auth")
	auth.Use(servermiddleware.BackendModeAuthGuard(settingService))
	{
		// 注册/登录/2FA/验证码发送均属于高风险入口，增加服务端兜底限流（Redis 故障时 fail-close）
		auth.POST("/register", rateLimiter.LimitWithOptions("auth-register", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.Register)
		auth.POST("/login", rateLimiter.LimitWithOptions("auth-login", 20, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.Login)
		auth.POST("/login/2fa", rateLimiter.LimitWithOptions("auth-login-2fa", 20, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.Login2FA)
		auth.POST("/send-verify-code", rateLimiter.LimitWithOptions("auth-send-verify-code", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.SendVerifyCode)
		// Token刷新接口添加速率限制：每分钟最多 30 次（Redis 故障时 fail-close）
		auth.POST("/refresh", rateLimiter.LimitWithOptions("refresh-token", 30, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.RefreshToken)
		// 登出接口（公开，允许未认证用户调用以撤销Refresh Token）
		auth.POST("/logout", h.Auth.Logout)
		// 优惠码验证接口添加速率限制：每分钟最多 10 次（Redis 故障时 fail-close）
		auth.POST("/validate-promo-code", rateLimiter.LimitWithOptions("validate-promo", 10, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ValidatePromoCode)
		// 邀请码验证接口添加速率限制：每分钟最多 10 次（Redis 故障时 fail-close）
		auth.POST("/validate-invitation-code", rateLimiter.LimitWithOptions("validate-invitation", 10, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ValidateInvitationCode)
		// 忘记密码接口添加速率限制：每分钟最多 5 次（Redis 故障时 fail-close）
		auth.POST("/forgot-password", rateLimiter.LimitWithOptions("forgot-password", 5, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ForgotPassword)
		// 重置密码接口添加速率限制：每分钟最多 10 次（Redis 故障时 fail-close）
		auth.POST("/reset-password", rateLimiter.LimitWithOptions("reset-password", 10, time.Minute, middleware.RateLimitOptions{
			FailureMode: middleware.RateLimitFailClose,
		}), h.Auth.ResetPassword)
		auth.GET("/oauth/linuxdo/start", h.Auth.LinuxDoOAuthStart)
		auth.GET("/oauth/linuxdo/callback", h.Auth.LinuxDoOAuthCallback)
		auth.POST("/oauth/linuxdo/complete-registration",
			rateLimiter.LimitWithOptions("oauth-linuxdo-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CompleteLinuxDoOAuthRegistration,
		)
		auth.GET("/oauth/oidc/start", h.Auth.OIDCOAuthStart)
		auth.GET("/oauth/oidc/callback", h.Auth.OIDCOAuthCallback)
		auth.POST("/oauth/oidc/complete-registration",
			rateLimiter.LimitWithOptions("oauth-oidc-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}),
			h.Auth.CompleteOIDCOAuthRegistration,
		)
	}

	// Session-based browser auth endpoints used by the control-auth BFF.
	bff := v1.Group("")
	bff.Use(servermiddleware.BackendModeAuthGuard(settingService))
	{
		bff.GET("/bootstrap", h.Auth.Bootstrap)
		bff.GET("/jwks", h.Auth.JWKS)

		session := bff.Group("/session")
		{
			session.POST("/login", rateLimiter.LimitWithOptions("control-session-login", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.SessionLogin)
			session.POST("/login/totp", rateLimiter.LimitWithOptions("control-session-login-totp", 20, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.SessionLoginTOTP)
			session.POST("/logout", h.Auth.SessionLogout)
			session.POST("/refresh", rateLimiter.LimitWithOptions("control-session-refresh", 30, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.SessionRefresh)
		}

		registration := bff.Group("/registration")
		{
			registration.POST("/preflight", rateLimiter.LimitWithOptions("control-registration-preflight", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.RegistrationPreflight)
			registration.POST("/email-code", rateLimiter.LimitWithOptions("control-registration-email-code", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.RegistrationEmailCode)
			registration.POST("", rateLimiter.LimitWithOptions("control-registration", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.Registration)
			registration.POST("/complete", rateLimiter.LimitWithOptions("control-registration-complete", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.RegistrationComplete)
		}

		password := bff.Group("/password")
		{
			password.POST("/forgot", rateLimiter.LimitWithOptions("control-password-forgot", 5, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.PasswordForgot)
			password.POST("/reset", rateLimiter.LimitWithOptions("control-password-reset", 10, time.Minute, middleware.RateLimitOptions{
				FailureMode: middleware.RateLimitFailClose,
			}), h.Auth.PasswordReset)
		}

		oauth := bff.Group("/oauth")
		{
			oauth.GET("/:provider/start", h.Auth.OAuthStart)
			oauth.GET("/:provider/callback", h.Auth.OAuthCallback)
		}
	}

	// Authenticated BFF self-service routes. The handlers validate the control session from
	// cookies or Authorization headers directly because they do not use the legacy JWT surface.
	selfService := v1.Group("")
	{
		session := selfService.Group("/session")
		{
			session.POST("/logout-all", h.Auth.SessionsLogoutAll)
		}

		me := selfService.Group("/me")
		{
			me.PATCH("", h.Auth.PatchMe)
			me.POST("/password", h.Auth.ChangeMyPassword)
			me.POST("/embed-token", h.Auth.EmbedToken)

			totp := me.Group("/mfa/totp")
			{
				totp.GET("", h.Auth.GetMyTOTP)
				totp.POST("/send-code", h.Auth.SendMyTOTPCode)
				totp.POST("/setup", h.Auth.SetupMyTOTP)
				totp.POST("/enable", h.Auth.EnableMyTOTP)
				totp.POST("/disable", h.Auth.DisableMyTOTP)
			}
		}
	}

	// 公开设置（无需认证）
	settings := v1.Group("/settings")
	{
		settings.GET("/public", h.Setting.GetPublicSettings)
	}

	// 需要认证的当前用户信息
	authenticated := v1.Group("")
	authenticated.Use(gin.HandlerFunc(jwtAuth))
	authenticated.Use(servermiddleware.BackendModeUserGuard(settingService))
	{
		authenticated.GET("/auth/me", h.Auth.GetCurrentUser)
		// 撤销所有会话（需要认证）
		authenticated.POST("/auth/revoke-all-sessions", h.Auth.RevokeAllSessions)
	}
}
