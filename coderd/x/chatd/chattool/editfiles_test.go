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

	// The result claims nothing was applied only when the agent's
	// response proves it: the agent returns 400 and 404 only before it
	// writes, and a single file is left untouched on any failure. Its
	// write phase can fail with 403 or 500 after committing earlier
	// files, and a transport error may follow a completed write.
	t.Run("AgentErrorResult", func(t *testing.T) {
		t.Parallel()
		const (
			oneFile  = `{"edits":[{"path":"/repo/a.go","old_text":"old","new_text":"new"}]}`
			twoFiles = `{"edits":[{"path":"/repo/a.go","old_text":"old","new_text":"new"},{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"}]}`
		)
		agentError := func(status int, message string) error {
			sdkErr := codersdk.NewTestError(status, "POST", "http://[fd7a::1]:4/api/v0/edit-files")
			sdkErr.Message = message
			return sdkErr
		}
		detailed := codersdk.NewTestError(http.StatusBadRequest, "POST", "http://[fd7a::1]:4/api/v0/edit-files")
		detailed.Message = `file path must be absolute: "a.txt"`
		detailed.Helper = "Use an absolute path."
		detailed.Detail = "some detail"
		detailed.Validations = []codersdk.ValidationError{{Field: "path", Detail: "must be absolute"}}
		tests := []struct {
			name     string
			input    string
			agentErr error
			wantErr  string
		}{
			{
				// Transport metadata from codersdk.Error.Error() is
				// dropped; the agent's message, helper, detail and
				// validations are kept.
				name:     "BadRequestOmitsTransportNoise",
				input:    `{"edits":[{"path":"a.txt","old_text":"old","new_text":"new"}]}`,
				agentErr: xerrors.Errorf("do request: %w", detailed),
				wantErr:  "No files were applied. file path must be absolute: \"a.txt\": Use an absolute path.: some detail\n- path: must be absolute\nFix the failing edit and resend all edits.",
			},
			{
				name:     "BadRequestSeveralFiles",
				input:    twoFiles,
				agentErr: agentError(http.StatusBadRequest, "edit /repo/b.go: search string not found in file"),
				wantErr:  "No files were applied. edit /repo/b.go: search string not found in file\nFix the failing edit and resend all edits.",
			},
			{
				name:     "NotFoundSeveralFiles",
				input:    twoFiles,
				agentErr: agentError(http.StatusNotFound, "open /repo/b.go: file does not exist"),
				wantErr:  "No files were applied. open /repo/b.go: file does not exist\nFix the failing edit and resend all edits.",
			},
			{
				name:     "ServerErrorOneFile",
				input:    oneFile,
				agentErr: agentError(http.StatusInternalServerError, "write /repo/a.go: no space left on device"),
				wantErr:  "No files were applied. write /repo/a.go: no space left on device\nFix the failing edit and resend all edits.",
			},
			{
				// Two edits to one file are still a single-file request.
				name:     "ServerErrorOneFileTwoEdits",
				input:    `{"edits":[{"path":"/repo/a.go","old_text":"old","new_text":"new"},{"path":"/repo/a.go","old_text":"foo()","new_text":"bar()"}]}`,
				agentErr: agentError(http.StatusInternalServerError, "write /repo/a.go: no space left on device"),
				wantErr:  "No files were applied. write /repo/a.go: no space left on device\nFix the failing edit and resend all edits.",
			},
			{
				name:     "ServerErrorSeveralFiles",
				input:    twoFiles,
				agentErr: agentError(http.StatusInternalServerError, "write /repo/b.go: no space left on device"),
				wantErr:  "It is unknown whether any files were applied. write /repo/b.go: no space left on device\nRe-read the files before resending edits.",
			},
			{
				name:     "ForbiddenSeveralFiles",
				input:    twoFiles,
				agentErr: agentError(http.StatusForbidden, "open /repo/.b.go.tmp.1234abcd: permission denied"),
				wantErr:  "It is unknown whether any files were applied. open /repo/.b.go.tmp.1234abcd: permission denied\nRe-read the files before resending edits.",
			},
			{
				name:     "TransportError",
				input:    oneFile,
				agentErr: xerrors.New("do request: connection reset by peer"),
				wantErr:  "It is unknown whether any files were applied. do request: connection reset by peer\nRe-read the files before resending edits.",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				ctrl := gomock.NewController(t)
				mockConn := agentconnmock.NewMockAgentConn(ctrl)
				mockConn.EXPECT().EditFiles(gomock.Any(), gomock.Any()).
					Return(workspacesdk.FileEditResponse{}, tt.agentErr)
				tool := chattool.EditFiles(chattool.EditFilesOptions{
					GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
						return mockConn, nil
					},
				})

				resp, err := tool.Run(context.Background(), fantasy.ToolCall{
					ID:    "call-1",
					Name:  "edit_files",
					Input: tt.input,
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
		assert.Equal(t, "Use the chat-specific absolute plan path; plan files must use absolute paths\nNo files were applied.", resp.Content)
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

func TestEditFiles_AppliedResult(t *testing.T) {
	t.Parallel()

	const (
		diffA = "--- /repo/a.go\n+++ /repo/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"
		diffB = "--- /repo/b.go\n+++ /repo/b.go\n@@ -1 +1 @@\n-foo()\n+bar()\n"
	)
	tests := []struct {
		name      string
		input     string
		agentResp workspacesdk.FileEditResponse
		want      string
	}{
		{
			name:  "OneFile",
			input: `{"edits":[{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"}]}`,
			agentResp: workspacesdk.FileEditResponse{Files: []workspacesdk.FileEditResult{
				{Path: "/repo/a.go", Diff: diffA},
			}},
			want: `{"status":"applied","message":"Applied edits to 1 file.","files":[` +
				`{"path":"/repo/a.go","status":"applied","diff":"--- /repo/a.go\n+++ /repo/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"}]}`,
		},
		{
			name: "SeveralFiles",
			input: `{"edits":[` +
				`{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"},` +
				`{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"}` +
				`]}`,
			agentResp: workspacesdk.FileEditResponse{Files: []workspacesdk.FileEditResult{
				{Path: "/repo/a.go", Diff: diffA},
				{Path: "/repo/b.go", Diff: diffB},
			}},
			want: `{"status":"applied","message":"Applied edits to 2 files.","files":[` +
				`{"path":"/repo/a.go","status":"applied","diff":"--- /repo/a.go\n+++ /repo/a.go\n@@ -1 +1 @@\n-x := 1\n+x := 2\n"},` +
				`{"path":"/repo/b.go","status":"applied","diff":"--- /repo/b.go\n+++ /repo/b.go\n@@ -1 +1 @@\n-foo()\n+bar()\n"}]}`,
		},
		{
			// An edit that changes nothing gets an empty diff from
			// the agent.
			name:  "EmptyDiff",
			input: `{"edits":[{"path":"/repo/a.go","old_text":"x","new_text":"x"}]}`,
			agentResp: workspacesdk.FileEditResponse{Files: []workspacesdk.FileEditResult{
				{Path: "/repo/a.go"},
			}},
			want: `{"status":"applied","message":"Applied edits to 1 file.","files":[{"path":"/repo/a.go","status":"applied","diff":""}]}`,
		},
		{
			// Agents that predate per-file results return none; the
			// count then comes from the files in the request, not the
			// edits.
			name: "AgentWithoutPerFileResults",
			input: `{"edits":[` +
				`{"path":"/repo/a.go","old_text":"x := 1","new_text":"x := 2"},` +
				`{"path":"/repo/b.go","old_text":"foo()","new_text":"bar()"},` +
				`{"path":"/repo/a.go","old_text":"y := 1","new_text":"y := 2"}` +
				`]}`,
			agentResp: workspacesdk.FileEditResponse{},
			want:      `{"status":"applied","message":"Applied edits to 2 files.","files":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			mockConn := agentconnmock.NewMockAgentConn(ctrl)
			mockConn.EXPECT().EditFiles(gomock.Any(), gomock.Any()).Return(tt.agentResp, nil)
			tool := chattool.EditFiles(chattool.EditFilesOptions{
				GetWorkspaceConn: func(context.Context) (workspacesdk.AgentConn, error) {
					return mockConn, nil
				},
			})

			resp, err := tool.Run(context.Background(), fantasy.ToolCall{
				ID:    "call-1",
				Name:  "edit_files",
				Input: tt.input,
			})
			require.NoError(t, err)
			assert.False(t, resp.IsError, resp.Content)
			assert.Equal(t, tt.want, resp.Content)
		})
	}
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
