package coderd

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

// chatFilesRateLimitMW returns the middleware enforcing FilesRateLimit
// on chat file routes. Both API prefixes mount the same instance, and
// the limiter keys on a prefix-stripped endpoint, so alternating
// prefixes cannot double the budget.
func (api *API) chatFilesRateLimitMW() func(http.Handler) http.Handler {
	api.chatFilesRateLimitOnce.Do(func() {
		api.chatFilesRateLimit = httpmw.RateLimitByAPICompatibilityEndpoint(api.FilesRateLimit, time.Minute)
	})
	return api.chatFilesRateLimit
}

// chatAPIPrefix identifies which API prefix a chat route mount serves.
type chatAPIPrefix int

const (
	chatAPIPrefixV2 chatAPIPrefix = iota
	chatAPIPrefixExperimental
)

// injectDefaultOrganizationParam lets the legacy default-organization
// routes reuse the organization-scoped handlers.
func injectDefaultOrganizationParam(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		chi.RouteContext(req.Context()).URLParams.Add("organization", codersdk.DefaultOrganization)
		next.ServeHTTP(rw, req)
	})
}

// registerChatAPIRoutes mounts the chat API surface on r, the root router
// of an API prefix. /api/v2 and /api/experimental serve the same promoted
// routes during the CODAGT-921 compatibility window. The experimental
// mount also serves the routes that were not promoted, while /api/v2
// reserves their path segments so they return 404 instead of falling into
// the {chat} wildcard.
// TODO(CODAGT-921): unmount from /api/experimental after the transition
// window (tracked in CODAGT-922).
func (api *API) registerChatAPIRoutes(r chi.Router, apiKeyMiddleware func(http.Handler) http.Handler, prefix chatAPIPrefix) {
	experimental := prefix == chatAPIPrefixExperimental
	// Signed URL tokens authenticate downloads, so the route stays
	// outside the API key middleware.
	r.Group(func(r chi.Router) {
		r.Use(api.chatFilesRateLimitMW())
		r.Get("/chats/files/{file}/download", api.downloadChatFile)
	})
	if experimental {
		// Superseded by the organization-scoped models collection and
		// deliberately not promoted. Keep until the frontend uses the
		// organization-scoped routes.
		r.Route("/chats/model-configs", func(r chi.Router) {
			r.Use(
				apiKeyMiddleware,
				injectDefaultOrganizationParam,
				httpmw.ExtractOrganizationParam(api.Database),
			)
			r.Get("/", api.listDefaultOrganizationChatModelConfigs)
			r.Post("/", api.createChatModelConfig)
		})
	}
	r.Route("/chats", func(r chi.Router) {
		r.Use(apiKeyMiddleware)
		if experimental {
			// Superseded by the organization-scoped models route and
			// deliberately not promoted. Keep until the frontend uses
			// the organization-scoped routes.
			r.With(
				injectDefaultOrganizationParam,
				httpmw.ExtractOrganizationParam(api.Database),
			).Get("/models", api.listChatModelConfigsByOrganization)
			// TODO(cian): place under /api/experimental/chats/config
			r.Route("/providers", func(r chi.Router) {
				r.Get("/", api.listChatProviders)
				r.Post("/", api.createChatProvider)
				r.Route("/{providerConfig}", func(r chi.Router) {
					r.Patch("/", api.updateChatProvider)
					r.Delete("/", api.deleteChatProvider)
				})
			})
			r.Route("/user-provider-configs", func(r chi.Router) {
				r.Get("/", api.listUserChatProviderConfigs)
				r.Route("/{providerConfig}", func(r chi.Router) {
					r.Put("/", api.upsertUserChatProviderKey)
					r.Delete("/", api.deleteUserChatProviderKey)
				})
			})
		} else {
			// These segments exist only under /api/experimental. Reserve
			// them with empty subrouters so they return 404 instead of
			// falling into the {chat} wildcard and failing UUID parsing
			// with a 400.
			// TODO(CODAGT-922): drop the reservations with the
			// experimental mounts.
			for _, segment := range []string{"/models", "/model-configs", "/providers", "/user-provider-configs"} {
				r.Route(segment, func(r chi.Router) {
					r.NotFound(func(rw http.ResponseWriter, _ *http.Request) {
						httpapi.RouteNotFound(rw)
					})
				})
			}
		}
		r.Get("/by-workspace", api.chatsByWorkspace)
		r.Get("/", api.listChats)
		r.Post("/", api.postChats)
		r.Get("/watch", api.watchChats)
		r.Route("/files", func(r chi.Router) {
			r.Use(api.chatFilesRateLimitMW())
			r.Post("/", api.postChatFile)
			r.Post("/{file}/download-url", api.postChatFileDownloadURL)
			r.Get("/{file}", api.chatFileByID)
		})
		r.Route("/config", func(r chi.Router) {
			r.Get("/system-prompt", api.getChatSystemPrompt)
			r.Put("/system-prompt", api.putChatSystemPrompt)
			r.Get("/plan-mode-instructions", api.getChatPlanModeInstructions)
			r.Put("/plan-mode-instructions", api.putChatPlanModeInstructions)
			r.Get("/personal-model-overrides", api.getChatPersonalModelOverridesAdminSettings)
			r.Put("/personal-model-overrides", api.putChatPersonalModelOverridesAdminSettings)
			r.Get("/debug-logging", api.getChatDebugLogging)
			r.Put("/debug-logging", api.putChatDebugLogging)
			r.Get("/user-debug-logging", api.getUserChatDebugLogging)
			r.Put("/user-debug-logging", api.putUserChatDebugLogging)
			r.Get("/user-prompt", api.getUserChatCustomPrompt)
			r.Put("/user-prompt", api.putUserChatCustomPrompt)
			r.Get("/user-compaction-thresholds", api.getUserChatCompactionThresholds)
			r.Put("/user-compaction-thresholds/{modelConfig}", api.putUserChatCompactionThreshold)
			r.Delete("/user-compaction-thresholds/{modelConfig}", api.deleteUserChatCompactionThreshold)
			r.Get("/workspace-ttl", api.getChatWorkspaceTTL)
			r.Put("/workspace-ttl", api.putChatWorkspaceTTL)
			r.Get("/retention-days", api.getChatRetentionDays)
			r.Put("/retention-days", api.putChatRetentionDays)
			r.Get("/debug-retention-days", api.getChatDebugRetentionDays)
			r.Put("/debug-retention-days", api.putChatDebugRetentionDays)
			r.Get("/auto-archive-days", api.getChatAutoArchiveDays)
			r.Put("/auto-archive-days", api.putChatAutoArchiveDays)
			if experimental {
				r.Group(func(r chi.Router) {
					r.Use(httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentChatVirtualDesktop))
					r.Get("/computer-use-provider", api.getChatComputerUseProvider)
					r.Put("/computer-use-provider", api.putChatComputerUseProvider)
				})
				r.Group(func(r chi.Router) {
					r.Use(httpmw.RequireExperimentWithDevBypass(api.Experiments, codersdk.ExperimentChatAdvisor))
					r.Get("/advisor", api.getChatAdvisorConfig)
					r.Put("/advisor", api.putChatAdvisorConfig)
				})
			}
		})
		r.Route("/{chat}", func(r chi.Router) {
			r.Use(httpmw.ExtractChatParam(api.Database))
			r.Route("/acl", func(r chi.Router) {
				r.Get("/", api.getChatACL)
				r.Patch("/", api.patchChatACL)
			})
			r.Get("/", api.getChat)
			r.Patch("/", api.patchChat)
			r.Get("/cost", api.getChatCost)
			r.Get("/messages", api.getChatMessages)
			r.Post("/messages", api.postChatMessages)
			r.Patch("/messages/{message}", api.patchChatMessage)
			r.Get("/prompts", api.getChatUserPrompts)
			r.Post("/interrupt", api.interruptChat)
			r.Post("/compact", api.compactChat)
			r.Post("/reconcile-invalid", api.reconcileInvalidChatState)
			r.Post("/tool-results", api.postChatToolResults)
			r.Post("/title/propose", api.proposeChatTitle)
			r.Get("/diff", api.getChatDiffContents)
			r.Put("/context", api.refreshChatContext)
			r.Route("/queue/{queuedMessage}", func(r chi.Router) {
				r.Delete("/", api.deleteChatQueuedMessage)
				r.Post("/promote", api.promoteChatQueuedMessage)
			})
			r.Route("/stream", func(r chi.Router) {
				r.Get("/", api.streamChat)
				r.Get("/parts", api.streamChatParts)
				r.Get("/git", api.watchChatGit)
				if experimental {
					r.Get("/desktop", api.watchChatDesktop)
				}
			})
			if experimental {
				r.Route("/debug", func(r chi.Router) {
					r.Get("/runs", api.getChatDebugRuns)
					r.Get("/runs/{debugRun}", api.getChatDebugRun)
				})
			}
		})
	})
}

// registerMCPServerOAuth2Routes mounts the user-scoped MCP server OAuth2
// routes shared by both API prefixes.
func (api *API) registerMCPServerOAuth2Routes(r chi.Router, prefix chatAPIPrefix) {
	if prefix == chatAPIPrefixExperimental {
		// Providers pin the redirect URI when a session is established,
		// so the callback URL cannot change for existing sessions without
		// breaking token refresh and forcing a re-auth.
		// TODO(CODAGT-922): promote once existing sessions can be
		// re-authenticated against a /api/v2 callback.
		r.Get("/servers/{mcpServer}/oauth2/callback", api.mcpServerOAuth2Callback)
	}
	// Disconnect stays outside organization routes so former organization
	// members can delete their stored token after losing config read access.
	r.Delete("/servers/{mcpServer}/oauth2/disconnect", api.mcpServerOAuth2Disconnect)
}

func (api *API) registerUserAIProviderKeyRoutes(r chi.Router) {
	r.Get("/", api.listUserAIProviderKeyConfigs)
	r.Route("/{aiProvider}", func(r chi.Router) {
		r.Put("/", api.upsertUserAIProviderKey)
		r.Delete("/", api.deleteUserAIProviderKey)
	})
}

// registerOrganizationChatRoutes mounts the organization-scoped chat and
// MCP server configuration routes; r must already extract the
// organization parameter.
func (api *API) registerOrganizationChatRoutes(r chi.Router, prefix chatAPIPrefix) {
	r.Route("/mcp-servers", func(r chi.Router) {
		r.Get("/", api.listMCPServerConfigs)
		r.Post("/", api.createMCPServerConfig)
		r.Route("/{mcpserverconfig}", func(r chi.Router) {
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionRead, policy.ActionUpdate, policy.ActionDelete)).Get("/", api.getMCPServerConfig)
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionUpdate)).Patch("/", api.updateMCPServerConfig)
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionDelete)).Delete("/", api.deleteMCPServerConfig)
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionShare)).Get("/acl", api.mcpServerConfigACL)
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionShare)).Patch("/acl", api.patchMCPServerConfigACL)
			if prefix == chatAPIPrefixV2 {
				r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
					policy.ActionShare)).Get("/acl/available", api.mcpServerConfigACLAvailable)
			}
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionRead)).Get("/oauth2/connect", api.mcpServerOAuth2Connect)
		})
	})
	r.Route("/chats/model-overrides", func(r chi.Router) {
		r.Get("/", api.getOrganizationChatModelOverrides)
		r.Put("/{context}", api.putOrganizationChatModelOverride)
	})
	r.Route("/chats/models", func(r chi.Router) {
		r.Get("/", api.listChatModelConfigsByOrganization)
		r.Post("/", api.createChatModelConfig)
		r.Route("/{model}", func(r chi.Router) {
			r.Use(httpmw.ExtractChatModelConfigParam(api.Database))
			r.Get("/", api.getChatModelConfig)
			r.Patch("/", api.updateChatModelConfig)
			r.Delete("/", api.deleteChatModelConfig)
			r.Route("/acl", func(r chi.Router) {
				r.Get("/", api.chatModelConfigACLHandler)
				r.Patch("/", api.updateChatModelConfigACL)
			})
		})
	})
}

func (api *API) registerOrganizationMemberChatRoutes(r chi.Router) {
	r.Route("/chats/model-overrides", func(r chi.Router) {
		r.Get("/", api.getUserChatPersonalModelOverrides)
		r.Put("/{context}", api.putUserChatPersonalModelOverride)
	})
}
