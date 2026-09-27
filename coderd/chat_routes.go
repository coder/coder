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

// registerChatAPIRoutes mounts the chat API surface on r, the /api/v2 root
// router.
func (api *API) registerChatAPIRoutes(r chi.Router, apiKeyMiddleware func(http.Handler) http.Handler) {
	// Signed URL tokens authenticate downloads, so the route stays
	// outside the API key middleware.
	r.Group(func(r chi.Router) {
		r.Use(httpmw.RateLimit(api.FilesRateLimit, time.Minute))
		r.Get("/chats/files/{file}/download", api.downloadChatFile)
	})
	r.Route("/chats", func(r chi.Router) {
		r.Use(apiKeyMiddleware)
		// Reserve the unmounted segment so it returns 404 instead of
		// falling into the {chat} wildcard and failing UUID parsing
		// with a 400.
		r.Route("/model-configs", func(r chi.Router) {
			r.NotFound(func(rw http.ResponseWriter, _ *http.Request) {
				httpapi.RouteNotFound(rw)
			})
		})
		r.Get("/by-workspace", api.chatsByWorkspace)
		r.Get("/", api.listChats)
		r.Post("/", api.postChats)
		r.Get("/watch", api.watchChats)
		r.Route("/files", func(r chi.Router) {
			r.Use(httpmw.RateLimit(api.FilesRateLimit, time.Minute))
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
			r.Post("/clear", api.clearChat)
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
			})
		})
	})
}

// registerExperimentalChatRoutes mounts the chat routes that were not
// promoted to /api/v2 on r, the /api/experimental root router.
func (api *API) registerExperimentalChatRoutes(r chi.Router, apiKeyMiddleware func(http.Handler) http.Handler) {
	r.Route("/chats", func(r chi.Router) {
		r.Use(apiKeyMiddleware)
		r.Route("/config", func(r chi.Router) {
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
		})
		r.Route("/{chat}", func(r chi.Router) {
			r.Use(httpmw.ExtractChatParam(api.Database))
			r.Get("/stream/desktop", api.watchChatDesktop)
			r.Route("/debug", func(r chi.Router) {
				r.Get("/runs", api.getChatDebugRuns)
				r.Get("/runs/{debugRun}", api.getChatDebugRun)
			})
		})
	})
}

func (api *API) registerUserAIProviderKeyRoutes(r chi.Router) {
	r.Get("/", api.listUserAIProviderKeyConfigs)
	r.Route("/{aiProvider}", func(r chi.Router) {
		r.Put("/", api.upsertUserAIProviderKey)
		r.Delete("/", api.deleteUserAIProviderKey)
		r.Route("/device-grants", func(r chi.Router) {
			r.Post("/", api.postUserAIDeviceGrant)
			r.Route("/{grant}", func(r chi.Router) {
				r.Get("/", api.getUserAIDeviceGrant)
				r.Delete("/", api.deleteUserAIDeviceGrant)
			})
		})
		r.Route("/browser-grants", func(r chi.Router) {
			r.Post("/", api.postUserAIBrowserGrant)
			r.Route("/{grant}", func(r chi.Router) {
				r.Post("/exchange", api.postUserAIBrowserGrantExchange)
				r.Delete("/", api.deleteUserAIBrowserGrant)
			})
		})
	})
}

// registerOrganizationChatRoutes mounts the organization-scoped chat and
// MCP server configuration routes; r must already extract the
// organization parameter.
func (api *API) registerOrganizationChatRoutes(r chi.Router) {
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
			r.With(httpmw.ExtractMCPServerConfigParam(api.Database, api.HTTPAuth.Authorize,
				policy.ActionShare)).Get("/acl/available", api.mcpServerConfigACLAvailable)
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
				r.Get("/available", api.chatModelConfigACLAvailable)
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
