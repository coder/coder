package coderd

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
)

func (api *API) registerChatFileDownloadRoute(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpmw.RateLimit(api.FilesRateLimit, time.Minute))
		r.Get("/chats/files/{file}/download", api.downloadChatFile)
	})
}

func (api *API) registerDefaultOrganizationChatModelsRoute(r chi.Router) {
	r.With(
		func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				chi.RouteContext(req.Context()).URLParams.Add("organization", codersdk.DefaultOrganization)
				next.ServeHTTP(rw, req)
			})
		},
		httpmw.ExtractOrganizationParam(api.Database),
	).Get("/models", api.listDefaultOrganizationChatModels)
}

func (api *API) registerUserAIProviderKeyRoutes(r chi.Router) {
	r.Get("/", api.listUserAIProviderKeyConfigs)
	r.Route("/{aiProvider}", func(r chi.Router) {
		r.Put("/", api.upsertUserAIProviderKey)
		r.Delete("/", api.deleteUserAIProviderKey)
	})
}

func (api *API) registerOrganizationMCPServerRoutes(r chi.Router) {
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
				policy.ActionRead)).Get("/oauth2/connect", api.mcpServerOAuth2Connect)
		})
	})
}

func (api *API) registerOrganizationChatRoutes(r chi.Router) {
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

func (api *API) registerChatCollectionRoutes(r chi.Router) {
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
}

func (api *API) registerChatConfigRoutes(r chi.Router) {
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
}

func (api *API) registerChatRoutes(r chi.Router) {
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
}

func (api *API) registerChatStreamRoutes(r chi.Router) {
	r.Get("/", api.streamChat)
	r.Get("/parts", api.streamChatParts)
	r.Get("/git", api.watchChatGit)
}

func (api *API) registerMCPServerOAuth2Routes(r chi.Router) {
	// This callback path is frozen because it is registered with OAuth2 providers.
	r.Get("/servers/{mcpServer}/oauth2/callback", api.mcpServerOAuth2Callback)
	// Disconnect stays outside organization routes so former organization
	// members can delete their stored token after losing config read access.
	r.Delete("/servers/{mcpServer}/oauth2/disconnect", api.mcpServerOAuth2Disconnect)
}
