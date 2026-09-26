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
				wantErr: "Set path to the absolute path of the file to edit in edits[1], edits[2]; path is required in every edit; no files in this batch were applied",
			},
			{
				name:    "EmptyEdits",
				input:   `{"edits":[]}`,
				wantErr: "Add at least one edit to edits; no files in this batch were applied",
			},
			{
				name:    "MissingEdits",
				input:   `{}`,
				wantErr: "Add at least one edit to edits; no files in this batch were applied",
			},
			{
				name:    "OldFilesShape",
				input:   `{"files":[{"path":"/repo/a.go","edits":[{"old_text":"old","new_text":"new"}]}]}`,
				wantErr: "Send a flat edits list where every edit has its own path, for example " + example + "; the files key is not supported; no files in this batch were applied",
			},
			{
				// fantasy's own decode error names Go types and does
				// not say that edits must be an array.
				name:  "EditsNotAnArray",
				input: `{"edits":"[{\"path\":\"/repo/a.go\"}]"}`,
				wantContains: []string{
					"Send edits as a JSON array of objects with string path, old_text and new_text and optional boolean replace_all, for example " + example,
					"no files in this batch were applied",
				},
			},
			{
				name:  "InputNotAnObject",
				input: `[]`,
				wantContains: []string{
					"Send edits as a JSON array of objects",
					"no files in this batch were applied",
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

	// Edits are grouped by trimmed path into one all-or-nothing agent
	// request: files in order of first appearance, each file's edits in
	// their original order.
	t.Run("GroupsEditsIntoOneRequest", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name  string
			input string
			want  []workspacesdk.FileEdits
		}{
			{
				name:  "SingleFile",
				input: `{"edits":[{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2","replace_all":true}]}`,
				want: []workspacesdk.FileEdits{
					{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{{OldText: "x := 1", NewText: "x := 2", ReplaceAll: true}}},
				},
			},
			{
				name: "MultipleFiles",
				input: `{"edits":[` +
					`{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"},` +
					`{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"}` +
					`]}`,
				want: []workspacesdk.FileEdits{
					{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{{OldText: "x := 1", NewText: "x := 2"}}},
					{Path: "/repo/b.go", Edits: []workspacesdk.FileEdit{{OldText: "foo()", NewText: "bar()"}}},
				},
			},
			{
				name: "InterleavedAndUntrimmedPaths",
				input: `{"edits":[` +
					`{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"},` +
					`{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"},` +
					`{"path":" /repo/a.go\n","old_text":"y := 1","new_text":"y := 2"}` +
					`]}`,
				want: []workspacesdk.FileEdits{
					{Path: "/repo/a.go", Edits: []workspacesdk.FileEdit{
						{OldText: "x := 1", NewText: "x := 2"},
						{OldText: "y := 1", NewText: "y := 2"},
					}},
					{Path: "/repo/b.go", Edits: []workspacesdk.FileEdit{{OldText: "foo()", NewText: "bar()"}}},
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)
				mockConn.EXPECT().
					EditFiles(gomock.Any(), workspacesdk.FileEditRequest{Files: tc.want, IncludeDiff: true}).
					Return(workspacesdk.FileEditResponse{}, nil).
					Times(1)
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
				assert.False(t, resp.IsError, resp.Content)
			})
		}
	})

	t.Run("AgentAPIErrorOmitsTransportNoise", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		mockConn := agentconnmock.NewMockAgentConn(ctrl)
		sdkErr := codersdk.NewTestError(http.StatusBadRequest, "POST", "http://[fd7a::1]:4/api/v0/edit-files")
		sdkErr.Message = `file path must be absolute: "a.txt"`
		sdkErr.Helper = "Use an absolute path."
		sdkErr.Detail = "some detail"
		sdkErr.Validations = []codersdk.ValidationError{{Field: "path", Detail: "must be absolute"}}
		mockConn.EXPECT().EditFiles(gomock.Any(), gomock.Any()).
			Return(workspacesdk.FileEditResponse{}, xerrors.Errorf("do request: %w", sdkErr))

		tool := chattool.EditFiles(chattool.EditFilesOptions{
			GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
				return mockConn, nil
			},
		})

		resp, err := tool.Run(context.Background(), fantasy.ToolCall{
			ID:    "call-1",
			Name:  "edit_files",
			Input: `{"edits":[{"path":"a.txt","old_text":"old","new_text":"new"}]}`,
		})
		require.NoError(t, err)
		assert.True(t, resp.IsError)
		assert.Equal(t, "file path must be absolute: \"a.txt\": Use an absolute path.: some detail\n- path: must be absolute", resp.Content)
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
		assert.Equal(t, "during plan turns, edit_files is restricted to "+planPath, resp.Content)
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
		assert.Equal(t, "during plan turns, edit_files is restricted to "+planPath, resp.Content)
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
		assert.Equal(t, "the chat-specific plan path /home/coder/.coder/plans/PLAN-test-uuid.md resolves to /home/coder/README.md; symlinked plan paths are not allowed during plan turns", resp.Content)
	})

	t.Run("RejectsPlanPathsWhenResolvePlanPathIsConfigured", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name                 string
			input                string
			expectedRejectedPath string
		}{
			{
				name:                 "SingleHomeRootPlanPath",
				input:                `{"edits":[{"path":"/Users/dev/plan.md","old_text":"old","new_text":"new"}]}`,
				expectedRejectedPath: "/Users/dev/plan.md",
			},
			{
				name: "MultiFileBatchWithHomeRootPlanPath",
				input: `{"edits":[` +
					`{"path":"/Users/dev/subdir/plan.md","old_text":"old","new_text":"new"},` +
					`{"path":"/Users/dev/plan.md","old_text":"old","new_text":"new"}` +
					`]}`,
				expectedRejectedPath: "/Users/dev/plan.md",
			},
		}

		for _, testCase := range tests {
			t.Run(testCase.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)
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
				assert.True(t, resp.IsError)
				assert.Equal(t, 1, resolvePlanPathCalls)
				assert.Equal(
					t,
					editFilesBatchRejectedMessage(sharedPlanPathResolvedMessage(
						testCase.expectedRejectedPath,
						"/Users/dev/.coder/plans/PLAN-chat.md",
					)),
					resp.Content,
				)
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
		assert.Equal(t, editFilesBatchRejectedMessage(planPathVerificationMessage("/home/coder/plan.md")), resp.Content)
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
		assert.Equal(t, editFilesBatchRejectedMessage(relativePlanPathMessage()), resp.Content)
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

func TestEditFiles_ToolResponseCarriesFileResults(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockConn := agentconnmock.NewMockAgentConn(ctrl)
	targetPath := "/home/coder/target.txt"
	expectedFiles := []workspacesdk.FileEditResult{
		{
			Path: targetPath,
			Diff: "--- " + targetPath + "\n+++ " + targetPath + "\n@@ -1 +1 @@\n-old\n+new\n",
		},
	}
	// The tool must opt into diffs (IncludeDiff: true) and forward
	// the agent's per-file results through to its response.
	mockConn.EXPECT().
		EditFiles(gomock.Any(), workspacesdk.FileEditRequest{
			Files: []workspacesdk.FileEdits{{
				Path: targetPath,
				Edits: []workspacesdk.FileEdit{{
					OldText: "old",
					NewText: "new",
				}},
			}},
			IncludeDiff: true,
		}).
		Return(workspacesdk.FileEditResponse{Files: expectedFiles}, nil)

	tool := chattool.EditFiles(chattool.EditFilesOptions{
		GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
			return mockConn, nil
		},
	})

	resp, err := tool.Run(context.Background(), fantasy.ToolCall{
		ID:    "call-1",
		Name:  "edit_files",
		Input: `{"edits":[{"path":"` + targetPath + `","old_text":"old","new_text":"new"}]}`,
	})
	require.NoError(t, err)
	assert.False(t, resp.IsError)

	var decoded struct {
		OK    bool                          `json:"ok"`
		Files []workspacesdk.FileEditResult `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(resp.Content), &decoded))
	assert.True(t, decoded.OK)
	require.Len(t, decoded.Files, 1)
	assert.Equal(t, targetPath, decoded.Files[0].Path)
	assert.Equal(t, expectedFiles[0].Diff, decoded.Files[0].Diff)
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
