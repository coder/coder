package xjson

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/asaskevich/govalidator"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func Test_convertValidationErrors(t *testing.T) {
	type args struct {
		errs govalidator.Errors
	}
	tests := []struct {
		name string
		args args
		want []m
	}{
		{"none", args{govalidator.Errors{}}, nil},
		// TODO (AB): Add more tests.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertValidationErrors(tt.args.errs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertValidationErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteErrPage(t *testing.T) {
	for _, test := range []struct {
		name string
		p    ErrPage
	}{
		{
			name: "OK",
			p: ErrPage{
				DevURL:    "ok-expected-dev-url",
				AccessURL: "ok-expected-access-url",
				Msg:       "ok-expected-msg",
				Code:      http.StatusBadGateway,
				Err:       xerrors.New("ok-expected-err"),
			},
		},
		{
			name: "Code Unset",
			p: ErrPage{
				DevURL:    "code-unset-expected-dev-url",
				AccessURL: "code-unset-expected-access-url",
				Msg:       "code-unset-expected-msg",
				Code:      0,
				Err:       xerrors.New("code-unset-expected-err"),
			},
		},
		{
			name: "Msg Unset",
			p: ErrPage{
				DevURL:    "msg-unset-expected-dev-url",
				AccessURL: "msg-unset-expected-access-url",
				Msg:       "",
				Code:      http.StatusInternalServerError,
				Err:       xerrors.New("msg-unset-expected-err"),
			},
		},
		{
			name: "AccessURL Unset",
			p: ErrPage{
				DevURL:    "access-url-unset-expected-dev-url",
				AccessURL: "",
				Msg:       "access-url-unset-expected-msg",
				Code:      http.StatusInternalServerError,
				Err:       xerrors.New("access-url-unset-expected-err"),
			},
		},
		{
			name: "DevURL Unset",
			p: ErrPage{
				DevURL:    "",
				AccessURL: "dev-url-unset-expected-access-url",
				Msg:       "dev-url-unset-expected-msg",
				Code:      http.StatusInternalServerError,
				Err:       xerrors.New("dev-url-unset-expected-err"),
			},
		},
		{
			name: "Nil Err",
			p: ErrPage{
				DevURL:    "nil-err-expected-dev-url",
				AccessURL: "nil-err-expected-access-url",
				Msg:       "nil-err-expected-msg",
				Code:      http.StatusInternalServerError,
				Err:       nil,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					WriteErrPage(w, test.p)
				},
			)

			s := httptest.NewServer(handler)
			defer s.Close()

			req := httptest.NewRequest(http.MethodGet, s.URL, nil)
			respRecorder := httptest.NewRecorder()
			handler(respRecorder, req)

			resp := respRecorder.Result()
			require.NotNil(t, resp, "expected non-nil response")
			defer resp.Body.Close()
			require.Equal(t, http.StatusInternalServerError, resp.StatusCode, "status codes differ")

			got, err := io.ReadAll(resp.Body)
			require.Equal(t, nil, err, "read body")

			switch test.name {
			case "OK":
				require.Equal(t, expectedOKErrPage(t), got, "OK response data does not match")
			case "Code Unset":
				require.Equal(t, expectedCodeUnsetErrPage(t), got, "code-unset response data does not match")
			case "Msg Unset":
				require.Equal(t, expectedMsgUnsetErrPage(t), got, "msg-unset response data does not match")
			case "DevURL Unset":
				require.Equal(t, expectedDevURLUnsetErrPage(t), got, "dev-url-unset response data does not match")
			case "AccessURL Unset":
				require.Equal(t, expectedAccessURLUnsetErrPage(t), got, "access-url-unset response data does not match")
			case "Nil Err":
				require.Equal(t, expectedNilErrPage(t), got, "nil-err response data does not match")
			default:
				t.Fail()
			}
		},
		)
	}
}

func toTemplateData(t *testing.T, ep ErrPage) []byte {
	view, err := template.New("").Funcs(
		template.FuncMap{
			"status": func(p ErrPage) string {
				return fmt.Sprintf("%d - %s", p.Code, http.StatusText(p.Code))
			},
		},
	).Parse(errPageMarkup)

	require.NoError(t, err)
	b := bytes.NewBuffer(nil)
	// We can use a bytes.Buffer as replacement for the http.ResponseWriter because
	// it implements the io.Writer interface.
	// Write the ErrPage to the template then writes the template to the buffer.
	require.NoError(t, view.ExecuteTemplate(b, "", ep))
	return b.Bytes()
}

func expectedOKErrPage(t *testing.T) []byte {
	// when this is turned into template data,
	// the retry button should be rendered by the handler
	// and match accordingly.
	return toTemplateData(t, ErrPage{
		DevURL:    "code-unset-expected-dev-url",
		AccessURL: "code-unset-expected-access-url",
		Msg:       "code-unset-expected-msg",
		Code:      http.StatusBadGateway,
		Err:       xerrors.New("code-unset-expected-err"),
	})
}

func expectedCodeUnsetErrPage(t *testing.T) []byte {
	return toTemplateData(t, ErrPage{
		DevURL:    "code-unset-expected-dev-url",
		AccessURL: "code-unset-expected-access-url",
		Msg:       "code-unset-expected-msg",
		// If Code is unset, it should default to a 500.
		Code: http.StatusInternalServerError,
		Err:  xerrors.New("code-unset-expected-err"),
	})
}

func expectedMsgUnsetErrPage(t *testing.T) []byte {
	return toTemplateData(t, ErrPage{
		DevURL:    "msg-unset-expected-dev-url",
		AccessURL: "msg-unset-expected-access-url",
		// If Msg was unset, it should default to the status text
		// of its status code..
		Msg:  http.StatusText(http.StatusInternalServerError),
		Code: http.StatusInternalServerError,
		Err:  xerrors.New("msg-unset-expected-err"),
	})
}

func expectedAccessURLUnsetErrPage(t *testing.T) []byte {
	return toTemplateData(t, ErrPage{
		DevURL: "access-url-unset-expected-dev-url",
		// AccessURL's do not get auto-corrected.
		// 'Back to Site' button will not work when
		// writing an ErrPage with an unset AccessURL
		// to a template.
		AccessURL: "",
		Msg:       "access-url-unset-expected-msg",
		Code:      http.StatusInternalServerError,
		Err:       xerrors.New("access-url-unset-expected-err"),
	})
}

func expectedNilErrPage(t *testing.T) []byte {
	return toTemplateData(t, ErrPage{
		DevURL:    "nil-err-expected-dev-url",
		AccessURL: "nil-err-expected-access-url",
		Msg:       "nil-err-expected-msg",
		Code:      http.StatusInternalServerError,
		Err:       nil,
	})
}

func expectedDevURLUnsetErrPage(t *testing.T) []byte {
	return toTemplateData(t, ErrPage{
		// The DevURL remains unadjusted because no logic
		// will adjust it. When the err page is turned into
		// template data the retry button won't be present.
		// The test server handler should do the same and provide
		// the expected result.
		DevURL:    "",
		AccessURL: "nil-err-expected-access-url",
		Msg:       "nil-err-expected-msg",
		Code:      http.StatusInternalServerError,
		Err:       nil,
	})
}
