package chattool_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"charm.land/fantasy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk/agentconnmock"
)

func TestEditFiles(t *testing.T) {
	t.Parallel()

	// The generated schema is the model-facing contract: a flat edits
	// list whose items carry their own path. fantasy cannot express
	// minItems, so "at least one edit" is enforced by validation.
	t.Run("SchemaIsFlatEditsList", func(t *testing.T) {
		t.Parallel()
		info := chattool.EditFiles(chattool.EditFilesOptions{}).Info()

		parameters, err := json.Marshal(info.Parameters)
		require.NoError(t, err)
		assert.JSONEq(t, `{"edits":{"type":"array","items":{
			"type":"object","required":["path","old_text","new_text"],"properties":{
				"path":{"type":"string","description":"Absolute path of the file to edit."},
				"old_text":{"type":"string","description":"Exact text to replace. Must match exactly one location unless replace_all is true. Must differ from new_text."},
				"new_text":{"type":"string","description":"Replacement text."},
				"replace_all":{"type":"boolean","description":"Replace every match of old_text."}}}}}`,
			string(parameters))
		assert.Equal(t, []string{"edits"}, info.Required)
		assert.Contains(t, info.Description, "Each file's edits are validated before that file is written: a file with any error is left unchanged, and the other files are still applied.")
		assert.NotContains(t, info.Description, "All edits in a batch")
	})

	t.Run("RejectedInputNamesWhatToChange", func(t *testing.T) {
		t.Parallel()
		const example = `{"edits":[{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"},{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"}]}`
		cases := []struct {
			name         string
			input        string
			wantErr      string
			wantContains []string
		}{
			{
				name: "EveryMissingPathListed",
				input: `{"edits":[` +
					`{"path":"/repo/a.go","old_text":"old","new_text":"new"},` +
					`{"old_text":"old","new_text":"new"},` +
					`{"path":"  ","old_text":"old","new_text":"new"}` +
					`]}`,
				wantErr: "Set path to the absolute path of the file to edit in edits[1], edits[2]; path is required in every edit\nNo files were applied.",
			},
			{
				name:    "EmptyEdits",
				input:   `{"edits":[]}`,
				wantErr: "Add at least one edit to edits\nNo files were applied.",
			},
			{
				name:    "MissingEdits",
				input:   `{}`,
				wantErr: "Add at least one edit to edits\nNo files were applied.",
			},
			{
				name:    "OldFilesShape",
				input:   `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"old","new_text":"new"}]}]}`,
				wantErr: "Send a flat edits list where every edit has its own path, for example " + example + "; the files key is not supported\nNo files were applied.",
			},
			{
				// fantasy's own decode error names Go types and does
				// not say that edits must be an array. chatloop decodes
				// a string holding an array before the tool runs, so
				// this string holds something else.
				name:  "EditsNotAnArray",
				input: `{"edits":"not json"}`,
				wantContains: []string{
					"Send edits as a JSON array of objects with string path, old_text and new_text and optional boolean replace_all, for example " + example,
					"\nNo files were applied.",
				},
			},
			{
				name:  "InputNotAnObject",
				input: `[]`,
				wantContains: []string{
					"Send edits as a JSON array of objects",
					"\nNo files were applied.",
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)
				tool := chattool.EditFiles(chattool.EditFilesOptions{
					GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
						return mockConn, nil
					},
				})

				resp, err := tool.Run(context.Background(), fantasy.ToolCall{
					ID:    "call-1",
					Name:  "edit_files",
					Input: tc.input,
				})
				require.NoError(t, err)
				assert.True(t, resp.IsError)
				if tc.wantErr != "" {
					assert.Equal(t, tc.wantErr, resp.Content)
				}
				for _, want := range tc.wantContains {
					assert.Contains(t, resp.Content, want)
				}
			})
		}
	})

	// Failures before the edit request reaches the agent write nothing,
	// so the result says so.
	t.Run("WorkspaceUnavailableAppliesNothing", func(t *testing.T) {
		t.Parallel()
		const input = `{"edits":[{"path":"/repo/a.go","old_text":"old","new_text":"new"}]}`
		tests := []struct {
			name    string
			options chattool.EditFilesOptions
			wantErr string
		}{
			{
				name:    "ResolverNotConfigured",
				options: chattool.EditFilesOptions{},
				wantErr: "workspace connection resolver is not configured\nNo files were applied.",
			},
			{
				name: "ConnectionFails",
				options: chattool.EditFilesOptions{
					GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
						return nil, xerrors.New("workspace agent is not connected")
					},
				},
				wantErr: "workspace agent is not connected\nNo files were applied.",
			},
			{
				name: "PlanPathResolveFails",
				options: chattool.EditFilesOptions{
					ResolvePlanPath: func(context.Context) (string, string, error) {
						return "", "", xerrors.New("workspace unavailable")
					},
					IsPlanTurn: true,
				},
				wantErr: "resolve chat-specific plan path: workspace unavailable\nNo files were applied.",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				resp, err := chattool.EditFiles(tt.options).Run(context.Background(), fantasy.ToolCall{
					ID:    "call-1",
					Name:  "edit_files",
					Input: input,
				})
				require.NoError(t, err)
				assert.True(t, resp.IsError)
				assert.Equal(t, tt.wantErr, resp.Content)
			})
		}
	})

	t.Run("PlanTurnRejectsNonPlanPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		getWorkspaceConnCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				getWorkspaceConnCalled = true
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"/home/coder/README.md","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "Edit only "+planPath+"; during plan turns, edit_files is restricted to that file\nNo files were applied.", resp.Content)
		assert.False(t, getWorkspaceConnCalled)
	})

	t.Run("PlanTurnRejectsMixedPaths", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		getWorkspaceConnCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				getWorkspaceConnCalled = true
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:   "call-1",
			Name: "edit_files",
			Input: `{"edits":[` +
				`{"path":"` + planPath + `","old_text":"old","new_text":"new"},` +
				`{"path":"/home/coder/README.md","old_text":"old","new_text":"new"}` +
				`]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "Edit only "+planPath+"; during plan turns, edit_files is restricted to that file\nNo files were applied.", resp.Content)
		assert.False(t, getWorkspaceConnCalled)
	})

	t.Run("PlanTurnAllowsResolvedPlanPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		resolvePlanPathCalls := 0
		mockConn.EXPECT().ResolvePath(gomock.Any(), planPath).Return(planPath, nil)
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: planPath,
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalls++
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"` + planPath + `","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.Equal(t, 1, resolvePlanPathCalls)
	})

	t.Run("PlanTurnAllowsLegacyAgentWithoutResolvePath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		mockConn.EXPECT().
			ResolvePath(gomock.Any(), planPath).
			Return("", statusError{statusCode: http.StatusNotFound, message: "missing resolve-path endpoint"})
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: planPath,
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"` + planPath + `","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})

	t.Run("PlanTurnRejectsSymlinkedPlanPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		planPath := "/home/coder/.coder/plans/PLAN-test-uuid.md"
		mockConn.EXPECT().ResolvePath(gomock.Any(), planPath).Return("/home/coder/README.md", nil)
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return planPath, "/home/coder", nil
			},
			IsPlanTurn: true,
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"` + planPath + `","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "the chat-specific plan path /home/coder/.coder/plans/PLAN-test-uuid.md resolves to /home/coder/README.md; symlinked plan paths are not allowed during plan turns\nNo files were applied.", resp.Content)
	})

	t.Run("RejectsPlanPathsWhenResolvePlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name  string
			input string
			// sentPath is a file in the batch that passes the check
			// and is sent to the agent; empty means none.
			sentPath    string
			wantIsError bool
			want        string
		}{
			{
				name:        "SingleHomeRootPlanPath",
				input:       `{"edits":[{"path":"/Users/dev/plan.md","old_text":"old","new_text":"new"}]}`,
				wantIsError: true,
				want: editFilesFileRejectedMessage("/Users/dev/plan.md", 0, sharedPlanPathResolvedMessage(
					"/Users/dev/plan.md",
					"/Users/dev/.coder/plans/PLAN-chat.md",
				)),
			},
			{
				name: "MultiFileBatchWithHomeRootPlanPath",
				input: `{"edits":[` +
					`{"path":"/Users/dev/subdir/plan.md","old_text":"old","new_text":"new"},` +
					`{"path":"/Users/dev/plan.md","old_text":"old","new_text":"new"}` +
					`]}`,
				sentPath: "/Users/dev/subdir/plan.md",
				want: `{"status":"partial",` +
					`"message":"Applied 1 file. /Users/dev/plan.md was not applied (edits[1] was not applied): fix and resend only the edits for /Users/dev/plan.md.",` +
					`"files":[` +
					`{"path":"/Users/dev/plan.md","status":"rejected","edits":[1],"error":"` +
					sharedPlanPathResolvedMessage("/Users/dev/plan.md", "/Users/dev/.coder/plans/PLAN-chat.md") + `"},` +
					`{"path":"/Users/dev/subdir/plan.md","status":"applied","diff":""}]}`,
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)
				if testCase.sentPath != "" {
					mockConn.EXPECT().
						EditFiles(gomock.Any(), workspacesdk.FileEditRequest{
							Files:       []workspacesdk.FileEdits{{Path: testCase.sentPath, Edits: []workspacesdk.FileEdit{{OldText: "old", NewText: "new"}}}},
							IncludeDiff: true,
						}).
						Return(workspacesdk.FileEditResponse{Files: []workspacesdk.FileEditResult{{Path: testCase.sentPath}}}, nil).
						Times(1)
				}
				resolvePlanPathCalls := 0
				tool := chattool.EditFiles(chattool.EditFilesOptions{
					GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
						return mockConn, nil
					},
					ResolvePlanPath: func(context.Context) (string, string, error) {
						resolvePlanPathCalls++
						return "/Users/dev/.coder/plans/PLAN-chat.md", "/Users/dev", nil
					},
				})

				resp, err := tool.Run(context.Background(), fantasy.ToolCall{
					ID:    "call-1",
					Name:  "edit_files",
					Input: testCase.input,
				})
				require.NoError(t, err)
				assert.Equal(t, testCase.wantIsError, resp.IsError, resp.Content)
				assert.Equal(t, 1, resolvePlanPathCalls)
				assert.Equal(t, testCase.want, resp.Content)
			})
		}
	})

	t.Run("RejectsSharedPlanPathWhenResolverFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return "", "", xerrors.New("workspace unavailable")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"/home/coder/plan.md","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, editFilesFileRejectedMessage("/home/coder/plan.md", 0, planPathVerificationMessage("/home/coder/plan.md")), resp.Content)
	})

	t.Run("RejectsRelativePlanPathsWhenResolvePlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		resolvePlanPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"plan.md","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
		assert.Equal(t, editFilesFileRejectedMessage("plan.md", 0, "Use the chat-specific absolute plan path; plan files must use absolute paths"), resp.Content)
	})

	t.Run("PerChatPlanPathIsAllowed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		chatPlanPath := "/home/coder/.coder/plans/PLAN-123e4567-e89b-12d3-a456-426614174000.md"
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: chatPlanPath,
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		resolvePlanPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return chatPlanPath, "/home/coder", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"` + chatPlanPath + `","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
	})

	t.Run("NestedPlanPathAllowedWhenResolverFails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: "/home/coder/myproject/plan.md",
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				return "", "", xerrors.New("workspace unavailable")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"/home/coder/myproject/plan.md","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})

	t.Run("NestedPlanPathUnderHomeIsAllowed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: "/home/coder/myproject/plan.md",
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		planPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				planPathCalled = true
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"/home/coder/myproject/plan.md","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.True(t, planPathCalled)
	})

	t.Run("AllowsNonSharedPath", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: "/home/dev/my-plan.md",
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		resolvePlanPathCalled := false
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
			ResolvePlanPath: func(context.Context) (string, string, error) {
				resolvePlanPathCalled = true
				return "", "", xerrors.New("should not be called")
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"/home/dev/my-plan.md","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
		assert.False(t, resolvePlanPathCalled)
	})

	t.Run("AllowsSharedPlanPathWhenResolvePlanPathIsNil", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		request := workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: chattool.LegacySharedPlanPath,
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}
		mockConn.EXPECT().EditFiles(gomock.Any(), request).Return(workspacesdk.FileEditResponse{}, nil)

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"` + chattool.LegacySharedPlanPath + `","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError)
	})
}

func TestEditFiles_MapsOldTextNewTextToSDK(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	targetPath := "/home/coder/main.go"

	mockConn.EXPECT().
		EditFiles(gomock.Any(), workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: targetPath,
				Edits: []workspacesdk.FileEdit{{
					OldText: "old content",
					NewText: "new content",
				}},
			}},
			IncludeDiff: true,
		}).
		Return(workspacesdk.FileEditResponse{}, nil)

	tool := chattool.EditFiles(chattool.EditFilesOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "edit_files",
		Input: `{"edits":[{"path":"` + targetPath + `","old_text":"old content","new_text":"new content"}]}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)
}

// Edits are grouped by trimmed path and each file is sent in its own
// agent request, in order of first appearance. Each file's status
// comes from its own response, and indexes in results are flat
// edits[i] indexes of the call.
func TestEditFiles_PerFileRequests(t *testing.T) {
	t.Parallel()

	const (
		diffA = "--- /repo/a.go\n+++ /repo/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"
		diffB = "--- /repo/b.go\n+++ /repo/b.go\n@@ -1 +1 @@\n-foo()\n+bar()\n"
		// JSON-escaped forms of the diffs above.
		diffAJSON = `--- /repo/a.go\n+++ /repo/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n`
		diffBJSON = `--- /repo/b.go\n+++ /repo/b.go\n@@ -1 +1 @@\n-foo()\n+bar()\n`

		editA = `{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"}`
		editB = `{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"}`
		editC = `{"path":"/repo/c.go","old_text":"a","new_text":"b"}`
	)
	var (
		fileEditA = workspacesdk.FileEdit{OldText: "x := 1", NewText: "x := 2"}
		fileEditB = workspacesdk.FileEdit{OldText: "foo()", NewText: "bar()"}
		fileEditC = workspacesdk.FileEdit{OldText: "a", NewText: "b"}
	)
	applied := func(path, diff string) workspacesdk.FileEditResponse {
		return workspacesdk.FileEditResponse{Files: []workspacesdk.FileEditResult{{Path: path, Diff: diff}}}
	}
	agentError := func(status int, message string) error {
		sdkErr := codersdk.NewTestError(status, "POST", "http://[fd7a::1]:4/api/v0/edit-files")
		sdkErr.Message = message
		return xerrors.Errorf("do request: %w", sdkErr)
	}
	detailed := codersdk.NewTestError(http.StatusBadRequest, "POST", "http://[fd7a::1]:4/api/v0/edit-files")
	detailed.Message = `file path must be absolute: "a.txt"`
	detailed.Helper = "Use an absolute path."
	detailed.Detail = "some detail"

	// fileCall is one expected agent request and the agent's answer.
	type fileCall struct {
		path  string
		edits []workspacesdk.FileEdit
		resp  workspacesdk.FileEditResponse
		err   error
	}
	tests := []struct {
		name            string
		input           string
		resolvePlanPath func(context.Context) (string, string, error)
		calls           []fileCall
		wantIsError     bool
		want            string
	}{
		{
			name:  "OneFileApplied",
			input: `{"edits":[` + editA + `]}`,
			calls: []fileCall{{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, resp: applied("/repo/a.go", diffA)}},
			want:  `{"status":"applied","message":"Applied edits to 1 file.","files":[{"path":"/repo/a.go","status":"applied","diff":"` + diffAJSON + `"}]}`,
		},
		{
			name:  "SeveralFilesApplied",
			input: `{"edits":[` + editA + `,` + editB + `]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, resp: applied("/repo/a.go", diffA)},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}, resp: applied("/repo/b.go", diffB)},
			},
			want: `{"status":"applied","message":"Applied edits to 2 files.","files":[` +
				`{"path":"/repo/a.go","status":"applied","diff":"` + diffAJSON + `"},` +
				`{"path":"/repo/b.go","status":"applied","diff":"` + diffBJSON + `"}]}`,
		},
		{
			// An edit that changes nothing gets an empty diff from
			// the agent.
			name:  "EmptyDiff",
			input: `{"edits":[{"path":"/repo/a.go","old_text":"x","new_text":"x"}]}`,
			calls: []fileCall{{path: "/repo/a.go", edits: []workspacesdk.FileEdit{{OldText: "x", NewText: "x"}}, resp: applied("/repo/a.go", "")}},
			want:  `{"status":"applied","message":"Applied edits to 1 file.","files":[{"path":"/repo/a.go","status":"applied","diff":""}]}`,
		},
		{
			// Agents that predate per-file results return none, so
			// applied files carry no diff.
			name:  "AgentWithoutPerFileResults",
			input: `{"edits":[` + editA + `,` + editB + `,{"path":"/repo/a.go","old_text":"y := 1","new_text":"y := 2"}]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA, {OldText: "y := 1", NewText: "y := 2"}}},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}},
			},
			want: `{"status":"applied","message":"Applied edits to 2 files.","files":[` +
				`{"path":"/repo/a.go","status":"applied"},{"path":"/repo/b.go","status":"applied"}]}`,
		},
		{
			name:  "DuplicatePathsShareOneRequest",
			input: `{"edits":[` + editA + `,{"path":"/repo/a.go","old_text":"y := 1","new_text":"y := 2"}]}`,
			calls: []fileCall{{
				path:  "/repo/a.go",
				edits: []workspacesdk.FileEdit{fileEditA, {OldText: "y := 1", NewText: "y := 2"}},
				resp:  applied("/repo/a.go", diffA),
			}},
			want: `{"status":"applied","message":"Applied edits to 1 file.","files":[{"path":"/repo/a.go","status":"applied","diff":"` + diffAJSON + `"}]}`,
		},
		{
			// A failing first file does not stop later files, and its
			// entry lists every edit sent for it.
			name:  "FailingFileDoesNotBlockOthers",
			input: `{"edits":[` + editA + `,` + editB + `,{"path":"/repo/a.go","old_text":"y := 1","new_text":"y := 2"},` + editC + `]}`,
			calls: []fileCall{
				{
					path:  "/repo/a.go",
					edits: []workspacesdk.FileEdit{fileEditA, {OldText: "y := 1", NewText: "y := 2"}},
					err:   agentError(http.StatusBadRequest, "edit /repo/a.go: search string not found in file"),
				},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}, resp: applied("/repo/b.go", diffB)},
				{path: "/repo/c.go", edits: []workspacesdk.FileEdit{fileEditC}, resp: applied("/repo/c.go", "")},
			},
			want: `{"status":"partial",` +
				`"message":"Applied 2 files. /repo/a.go was not applied (none of edits[0], edits[2] were applied): fix and resend only the edits for /repo/a.go.",` +
				`"files":[` +
				`{"path":"/repo/a.go","status":"rejected","edits":[0,2],"error":"edit /repo/a.go: search string not found in file"},` +
				`{"path":"/repo/b.go","status":"applied","diff":"` + diffBJSON + `"},` +
				`{"path":"/repo/c.go","status":"applied","diff":""}]}`,
		},
		{
			// Interleaved and untrimmed paths group into one request
			// per file, and rejected files are listed first.
			name:  "InterleavedPathsMapToFlatIndexes",
			input: `{"edits":[` + editA + `,` + editB + `,{"path":" /repo/a.go\n","old_text":"y := 1","new_text":"y := 2"},{"path":"/repo/b.go","old_text":"baz()","new_text":"qux()"}]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA, {OldText: "y := 1", NewText: "y := 2"}}, resp: applied("/repo/a.go", diffA)},
				{
					path:  "/repo/b.go",
					edits: []workspacesdk.FileEdit{fileEditB, {OldText: "baz()", NewText: "qux()"}},
					err:   agentError(http.StatusBadRequest, "edit /repo/b.go: search string not found in file"),
				},
			},
			want: `{"status":"partial",` +
				`"message":"Applied 1 file. /repo/b.go was not applied (none of edits[1], edits[3] were applied): fix and resend only the edits for /repo/b.go.",` +
				`"files":[` +
				`{"path":"/repo/b.go","status":"rejected","edits":[1,3],"error":"edit /repo/b.go: search string not found in file"},` +
				`{"path":"/repo/a.go","status":"applied","diff":"` + diffAJSON + `"}]}`,
		},
		{
			// An error without an agent response leaves the file's
			// outcome unknown.
			name:  "TransportErrorIsUnknown",
			input: `{"edits":[` + editA + `,` + editB + `]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, err: xerrors.New("do request: connection reset by peer")},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}, resp: applied("/repo/b.go", diffB)},
			},
			want: `{"status":"partial",` +
				`"message":"Applied 1 file. It is unknown whether /repo/a.go was applied (edits[0]): re-read /repo/a.go before resending its edits.",` +
				`"files":[` +
				`{"path":"/repo/a.go","status":"unknown","edits":[0],"error":"do request: connection reset by peer"},` +
				`{"path":"/repo/b.go","status":"applied","diff":"` + diffBJSON + `"}]}`,
		},
		{
			name:  "RejectedAndUnknownBeforeApplied",
			input: `{"edits":[` + editA + `,` + editB + `,` + editC + `]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, resp: applied("/repo/a.go", diffA)},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}, err: agentError(http.StatusNotFound, "open /repo/b.go: file does not exist")},
				{path: "/repo/c.go", edits: []workspacesdk.FileEdit{fileEditC}, err: xerrors.New("decode response body: unexpected EOF")},
			},
			want: `{"status":"partial",` +
				`"message":"Applied 1 file. /repo/b.go was not applied (edits[1] was not applied): fix and resend only the edits for /repo/b.go. It is unknown whether /repo/c.go was applied (edits[2]): re-read /repo/c.go before resending its edits.",` +
				`"files":[` +
				`{"path":"/repo/b.go","status":"rejected","edits":[1],"error":"open /repo/b.go: file does not exist"},` +
				`{"path":"/repo/c.go","status":"unknown","edits":[2],"error":"decode response body: unexpected EOF"},` +
				`{"path":"/repo/a.go","status":"applied","diff":"` + diffAJSON + `"}]}`,
		},
		{
			// A single-file request writes nothing on any agent
			// error, including a write-phase 500.
			name:  "NothingApplied",
			input: `{"edits":[` + editA + `,` + editB + `]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, err: agentError(http.StatusInternalServerError, "write /repo/a.go: no space left on device")},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}, err: agentError(http.StatusNotFound, "open /repo/b.go: file does not exist")},
			},
			wantIsError: true,
			want: "No files were applied.\n" +
				"- /repo/a.go (edits[0]): write /repo/a.go: no space left on device\n" +
				"- /repo/b.go (edits[1]): open /repo/b.go: file does not exist",
		},
		{
			// Transport metadata from codersdk.Error.Error() is
			// dropped; the agent's message, helper and detail are kept.
			name:        "AgentErrorOmitsTransportNoise",
			input:       `{"edits":[{"path":"a.txt","old_text":"x := 1","new_text":"x := 2"}]}`,
			calls:       []fileCall{{path: "a.txt", edits: []workspacesdk.FileEdit{fileEditA}, err: xerrors.Errorf("do request: %w", detailed)}},
			wantIsError: true,
			want:        "No files were applied.\n- a.txt (edits[0]): file path must be absolute: \"a.txt\": Use an absolute path.: some detail",
		},
		{
			name:        "OnlyUnknown",
			input:       `{"edits":[` + editA + `]}`,
			calls:       []fileCall{{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, err: xerrors.New("do request: connection reset by peer")}},
			wantIsError: true,
			want: "No files were applied, except that files marked unknown may have been.\n" +
				"- /repo/a.go (edits[0]): unknown whether applied (do request: connection reset by peer); re-read it before resending its edits",
		},
		{
			name:  "RejectedAndUnknownNothingApplied",
			input: `{"edits":[` + editA + `,` + editB + `]}`,
			calls: []fileCall{
				{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, err: agentError(http.StatusBadRequest, "edit /repo/a.go: search string not found in file")},
				{path: "/repo/b.go", edits: []workspacesdk.FileEdit{fileEditB}, err: xerrors.New("do request: connection reset by peer")},
			},
			wantIsError: true,
			want: "No files were applied, except that files marked unknown may have been.\n" +
				"- /repo/a.go (edits[0]): edit /repo/a.go: search string not found in file\n" +
				"- /repo/b.go (edits[1]): unknown whether applied (do request: connection reset by peer); re-read it before resending its edits",
		},
		{
			// Path-tied coderd checks reject only their file, without
			// an agent request for it.
			name: "PathTiedChecksRejectOnlyTheirFile",
			input: `{"edits":[` +
				`{"path":"plan.md","old_text":"a","new_text":"b"},` +
				editA + `,` +
				`{"path":"/home/coder/plan.md","old_text":"c","new_text":"d"}` +
				`]}`,
			resolvePlanPath: func(context.Context) (string, string, error) {
				return "/home/coder/.coder/plans/PLAN-chat.md", "/home/coder", nil
			},
			calls: []fileCall{{path: "/repo/a.go", edits: []workspacesdk.FileEdit{fileEditA}, resp: applied("/repo/a.go", diffA)}},
			want: `{"status":"partial",` +
				`"message":"Applied 1 file. plan.md was not applied (edits[0] was not applied): fix and resend only the edits for plan.md. /home/coder/plan.md was not applied (edits[2] was not applied): fix and resend only the edits for /home/coder/plan.md.",` +
				`"files":[` +
				`{"path":"plan.md","status":"rejected","edits":[0],"error":"Use the chat-specific absolute plan path; plan files must use absolute paths"},` +
				`{"path":"/home/coder/plan.md","status":"rejected","edits":[2],"error":"the plan path /home/coder/plan.md is no longer supported at the home root; use the chat-specific plan path: /home/coder/.coder/plans/PLAN-chat.md"},` +
				`{"path":"/repo/a.go","status":"applied","diff":"` + diffAJSON + `"}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			expected := make([]any, 0, len(tt.calls))
			for _, call := range tt.calls {
				request := workspacesdk.FileEditRequest{
					Files:       []workspacesdk.FileEdits{{Path: call.path, Edits: call.edits}},
					IncludeDiff: true,
				}
				expected = append(expected, mockConn.EXPECT().
					EditFiles(gomock.Any(), request).
					Return(call.resp, call.err).
					Times(1))
			}
			gomock.InOrder(expected...)
			tool := chattool.EditFiles(chattool.EditFilesOptions{
				GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
					return mockConn, nil
				},
				ResolvePlanPath: tt.resolvePlanPath,
			})

			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-1",
				Name:  "edit_files",
				Input: tt.input,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantIsError, resp.IsError, resp.Content)
			assert.Equal(t, tt.want, resp.Content)
		})
	}

	// A result produced after cancellation can still be persisted, so
	// files not yet sent are reported as not applied rather than
	// unknown, and no request is made for them.
	t.Run("InterruptedBeforeLaterFiles", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		mockConn.EXPECT().
			EditFiles(gomock.Any(), workspacesdk.FileEditRequest{
				Files:       []workspacesdk.FileEdits{{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{fileEditA}}},
				IncludeDiff: true,
			}).
			DoAndReturn(func(context.Context, workspacesdk.FileEditRequest) (workspacesdk.FileEditResponse, error) {
				cancel()
				return applied("/repo/a.go", diffA), nil
			}).
			Times(1)
		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(ctx, fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[` + editA + `,` + editB + `]}`,
		})
		require.NoError(t, err)
		assert.False(t, resp.IsError, resp.Content)
		assert.Equal(t, `{"status":"partial",`+
			`"message":"Applied 1 file. /repo/b.go was not applied (edits[1] was not applied): fix and resend only the edits for /repo/b.go.",`+
			`"files":[`+
			`{"path":"/repo/b.go","status":"rejected","edits":[1],"error":"not sent because the tool call was interrupted"},`+
			`{"path":"/repo/a.go","status":"applied","diff":"`+diffAJSON+`"}]}`, resp.Content)
	})
}

func TestEditFiles_DecodeToolInput(t *testing.T) {
	t.Parallel()

	tool, ok := chattool.EditFiles(chattool.EditFilesOptions{}).(interface {
		DecodeToolInput(input string) (decoded string, ok bool)
	})
	require.True(t, ok, "edit_files must implement DecodeToolInput")

	tests := []struct {
		name  string
		input string
		// want is the decoded input; empty means not ok.
		want string
	}{
		{
			name:  "StringHoldingArrayOfObjects",
			input: `{"edits":"[{\"path\":\"/repo/a.go\",\"old_text\":\"x\",\"new_text\":\"y\"}]"}`,
			want:  `{"edits":[{"path":"/repo/a.go","old_text":"x","new_text":"y"}]}`,
		},
		{
			// The string content is spliced in verbatim so the
			// ambiguity check sees the model's key spelling, and
			// other members and whitespace are left untouched.
			name:  "ContentAndOtherMembersKeptVerbatim",
			input: `{"x": 1, "edits" : " [ {\"PATH\" : \"/repo/a.go\"} ]", "y":[true]}`,
			want:  `{"x": 1, "edits" :  [ {"PATH" : "/repo/a.go"} ], "y":[true]}`,
		},
		{
			name:  "StringHoldingEmptyArray",
			input: `{"edits":"[]"}`,
			want:  `{"edits":[]}`,
		},
		{
			name:  "StringHoldingInvalidJSON",
			input: `{"edits":"[{\"path\":"}`,
		},
		{
			name:  "StringHoldingObject",
			input: `{"edits":"{\"path\":\"/repo/a.go\"}"}`,
		},
		{
			name:  "StringHoldingArrayOfNonObjects",
			input: `{"edits":"[1,\"a\",null]"}`,
		},
		{
			name:  "StringHoldingNull",
			input: `{"edits":"null"}`,
		},
		{
			name:  "StringHoldingTrailingData",
			input: `{"edits":"[] []"}`,
		},
		{
			// Only JSON whitespace may surround the array: the content
			// is spliced in verbatim and must stay valid JSON.
			name:  "StringWithLeadingNoBreakSpace",
			input: `{"edits":"\u00a0[{\"path\":\"/a\"}]"}`,
		},
		{
			name:  "StringWithLeadingVerticalTab",
			input: `{"edits":"\u000b[{\"path\":\"/a\"}]"}`,
		},
		{
			name:  "StringWithTrailingNextLine",
			input: `{"edits":"[{\"path\":\"/a\"}]\u0085"}`,
		},
		{
			name:  "StringWithJSONWhitespace",
			input: `{"edits":" \t\r\n[ {\"path\":\"/a\"} ]\n"}`,
			want:  "{\"edits\": \t\r\n[ {\"path\":\"/a\"} ]\n}",
		},
		{
			name:  "EditsAlreadyArray",
			input: `{"edits":[{"path":"/repo/a.go","old_text":"x","new_text":"y"}]}`,
		},
		{
			name:  "NoEdits",
			input: `{"files":"[]"}`,
		},
		{
			// A repeated key is left for the ambiguity check to
			// reject.
			name:  "RepeatedEditsKey",
			input: `{"edits":"[]","edits":"[]"}`,
		},
		{
			name:  "CaseVariantKey",
			input: `{"Edits":"[]"}`,
		},
		{
			name:  "InputNotAnObject",
			input: `"[{\"path\":\"/repo/a.go\"}]"`,
		},
		{
			name:  "InvalidInput",
			input: `{"edits":"[]"`,
		},
		{
			name:  "TrailingDataAfterObject",
			input: `{"edits":"[]"} {}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tool.DecodeToolInput(tt.input)
			if tt.want == "" {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEditFiles_Grouping(t *testing.T) {
	t.Parallel()

	e1 := chattool.EditFilesEdit{Path: "/repo/a.go", OldText: "x := 1", NewText: "x := 2"}
	e2 := chattool.EditFilesEdit{Path: "/repo/b.go", OldText: "foo()", NewText: "bar()"}
	e3 := chattool.EditFilesEdit{Path: "/repo/a.go", OldText: "y := 1", NewText: "y := 2"}
	all := chattool.EditFilesEdit{Path: "/repo/a.go", OldText: "foo", NewText: "bar", ReplaceAll: true}

	tests := []struct {
		name  string
		edits []chattool.EditFilesEdit
		// agentFiles is the grouped agent request form.
		agentFiles []workspacesdk.FileEdits
		// hookJSON is the exact pre_tool_use tool_input.
		hookJSON string
		// flattened is the result of flattening hookJSON back to
		// schema B.
		flattened []chattool.EditFilesEdit
	}{
		{
			name:  "Empty",
			edits: nil,
			// An empty slice, not nil, so hooks see an array.
			agentFiles: []workspacesdk.FileEdits{},
			hookJSON:   `{"files":[]}`,
			flattened:  []chattool.EditFilesEdit{},
		},
		{
			name:  "SingleFile",
			edits: []chattool.EditFilesEdit{e1, e3},
			agentFiles: []workspacesdk.FileEdits{
				{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{
					{OldText: "x := 1", NewText: "x := 2"},
					{OldText: "y := 1", NewText: "y := 2"},
				}},
			},
			hookJSON:  `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"x := 1","new_text":"x := 2"},{"old_text":"y := 1","new_text":"y := 2"}]}]}`,
			flattened: []chattool.EditFilesEdit{e1, e3},
		},
		{
			name:  "SeveralFiles",
			edits: []chattool.EditFilesEdit{e1, e3, e2},
			agentFiles: []workspacesdk.FileEdits{
				{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{
					{OldText: "x := 1", NewText: "x := 2"},
					{OldText: "y := 1", NewText: "y := 2"},
				}},
				{Path: "/repo/b.go", Edits: []workspacesdk.FileEdit{
					{OldText: "foo()", NewText: "bar()"},
				}},
			},
			hookJSON:  `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"x := 1","new_text":"x := 2"},{"old_text":"y := 1","new_text":"y := 2"}]},{"path":"/repo/b.go","edits":[{"old_text":"foo()","new_text":"bar()"}]}]}`,
			flattened: []chattool.EditFilesEdit{e1, e3, e2},
		},
		{
			// Grouping keeps each file's edit order but not the
			// order of edits across files.
			name:  "InterleavedPaths",
			edits: []chattool.EditFilesEdit{e1, e2, e3},
			agentFiles: []workspacesdk.FileEdits{
				{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{
					{OldText: "x := 1", NewText: "x := 2"},
					{OldText: "y := 1", NewText: "y := 2"},
				}},
				{Path: "/repo/b.go", Edits: []workspacesdk.FileEdit{
					{OldText: "foo()", NewText: "bar()"},
				}},
			},
			hookJSON:  `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"x := 1","new_text":"x := 2"},{"old_text":"y := 1","new_text":"y := 2"}]},{"path":"/repo/b.go","edits":[{"old_text":"foo()","new_text":"bar()"}]}]}`,
			flattened: []chattool.EditFilesEdit{e1, e3, e2},
		},
		{
			name:  "ReplaceAll",
			edits: []chattool.EditFilesEdit{all, e1},
			agentFiles: []workspacesdk.FileEdits{
				{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{
					{OldText: "foo", NewText: "bar", ReplaceAll: true},
					{OldText: "x := 1", NewText: "x := 2"},
				}},
			},
			hookJSON:  `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"foo","new_text":"bar","replace_all":true},{"old_text":"x := 1","new_text":"x := 2"}]}]}`,
			flattened: []chattool.EditFilesEdit{all, e1},
		},
		{
			// Paths compare exactly: no trimming, no case folding.
			name: "PathsCompareExactly",
			edits: []chattool.EditFilesEdit{
				{Path: "/repo/a.go", OldText: "a", NewText: "b"},
				{Path: "/repo/A.go", OldText: "c", NewText: "d"},
				{Path: "/repo/a.go ", OldText: "e", NewText: "f"},
			},
			agentFiles: []workspacesdk.FileEdits{
				{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{{OldText: "a", NewText: "b"}}},
				{Path: "/repo/A.go", Edits: []workspacesdk.FileEdit{{OldText: "c", NewText: "d"}}},
				{Path: "/repo/a.go ", Edits: []workspacesdk.FileEdit{{OldText: "e", NewText: "f"}}},
			},
			hookJSON: `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"a","new_text":"b"}]},{"path":"/repo/A.go","edits":[{"old_text":"c","new_text":"d"}]},{"path":"/repo/a.go ","edits":[{"old_text":"e","new_text":"f"}]}]}`,
			flattened: []chattool.EditFilesEdit{
				{Path: "/repo/a.go", OldText: "a", NewText: "b"},
				{Path: "/repo/A.go", OldText: "c", NewText: "d"},
				{Path: "/repo/a.go ", OldText: "e", NewText: "f"},
			},
		},
		{
			name:  "IdenticalDuplicatesKept",
			edits: []chattool.EditFilesEdit{e1, e1},
			agentFiles: []workspacesdk.FileEdits{
				{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{
					{OldText: "x := 1", NewText: "x := 2"},
					{OldText: "x := 1", NewText: "x := 2"},
				}},
			},
			hookJSON:  `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"x := 1","new_text":"x := 2"},{"old_text":"x := 1","new_text":"x := 2"}]}]}`,
			flattened: []chattool.EditFilesEdit{e1, e1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.agentFiles, chattool.GroupEditsByPath(tt.edits))

			hookInput, err := json.Marshal(chattool.NewEditFilesHookInput(tt.edits))
			require.NoError(t, err)
			assert.Equal(t, tt.hookJSON, string(hookInput))

			var override chattool.EditFilesHookInput
			require.NoError(t, json.Unmarshal(hookInput, &override))
			assert.Equal(t, tt.flattened, override.Edits())
		})
	}

	// A grouped override must accept only old_text/new_text, not the
	// deprecated search/replace keys that workspacesdk.FileEdit
	// decodes for old agents.
	t.Run("OverrideIgnoresSearchReplace", func(t *testing.T) {
		t.Parallel()

		var override chattool.EditFilesHookInput
		err := json.Unmarshal([]byte(`{"files":[{"path":"/repo/a.go","edits":[{"search":"foo()","replace":"bar()"}]}]}`), &override)
		require.NoError(t, err)
		assert.Equal(t, []chattool.EditFilesEdit{{Path: "/repo/a.go"}}, override.Edits())
	})
}
