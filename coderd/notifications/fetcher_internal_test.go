package notifications
import (
	"errors"
	"context"
	"database/sql"
	"testing"
	"text/template"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"github.com/coder/coder/v2/coderd/database/dbmock"
)
func TestNotifier_FetchHelpers(t *testing.T) {
	t.Parallel()
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store:   dbmock,
			helpers: template.FuncMap{},
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("ACME Inc.", nil)
		dbmock.EXPECT().GetLogoURL(gomock.Any()).Return("https://example.com/logo.png", nil)
		ctx := context.Background()
		helpers, err := n.fetchHelpers(ctx)
		require.NoError(t, err)
		appName, ok := helpers["app_name"].(func() string)
		require.True(t, ok)
		require.Equal(t, "ACME Inc.", appName())
		logoURL, ok := helpers["logo_url"].(func() string)
		require.True(t, ok)
		require.Equal(t, "https://example.com/logo.png", logoURL())
	})
	t.Run("failed to fetch app name", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store:   dbmock,
			helpers: template.FuncMap{},
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("", errors.New("internal error"))
		ctx := context.Background()
		_, err := n.fetchHelpers(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "get application name")
	})
	t.Run("failed to fetch logo URL", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store:   dbmock,
			helpers: template.FuncMap{},
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("ACME Inc.", nil)
		dbmock.EXPECT().GetLogoURL(gomock.Any()).Return("", errors.New("internal error"))
		ctx := context.Background()
		_, err := n.fetchHelpers(ctx)
		require.ErrorContains(t, err, "get logo URL")
	})
}
func TestNotifier_FetchAppName(t *testing.T) {
	t.Parallel()
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("ACME Inc.", nil)
		ctx := context.Background()
		appName, err := n.fetchAppName(ctx)
		require.NoError(t, err)
		require.Equal(t, "ACME Inc.", appName)
	})
	t.Run("No rows", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("", sql.ErrNoRows)
		ctx := context.Background()
		appName, err := n.fetchAppName(ctx)
		require.NoError(t, err)
		require.Equal(t, notificationsDefaultAppName, appName)
	})
	t.Run("Empty string", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("", nil)
		ctx := context.Background()
		appName, err := n.fetchAppName(ctx)
		require.NoError(t, err)
		require.Equal(t, notificationsDefaultAppName, appName)
	})
	t.Run("internal error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetApplicationName(gomock.Any()).Return("", errors.New("internal error"))
		ctx := context.Background()
		_, err := n.fetchAppName(ctx)
		require.Error(t, err)
	})
}
func TestNotifier_FetchLogoURL(t *testing.T) {
	t.Parallel()
	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetLogoURL(gomock.Any()).Return("https://example.com/logo.png", nil)
		ctx := context.Background()
		logoURL, err := n.fetchLogoURL(ctx)
		require.NoError(t, err)
		require.Equal(t, "https://example.com/logo.png", logoURL)
	})
	t.Run("No rows", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetLogoURL(gomock.Any()).Return("", sql.ErrNoRows)
		ctx := context.Background()
		logoURL, err := n.fetchLogoURL(ctx)
		require.NoError(t, err)
		require.Equal(t, notificationsDefaultLogoURL, logoURL)
	})
	t.Run("Empty string", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetLogoURL(gomock.Any()).Return("", nil)
		ctx := context.Background()
		logoURL, err := n.fetchLogoURL(ctx)
		require.NoError(t, err)
		require.Equal(t, notificationsDefaultLogoURL, logoURL)
	})
	t.Run("internal error", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		dbmock := dbmock.NewMockStore(ctrl)
		n := &notifier{
			store: dbmock,
		}
		dbmock.EXPECT().GetLogoURL(gomock.Any()).Return("", errors.New("internal error"))
		ctx := context.Background()
		_, err := n.fetchLogoURL(ctx)
		require.Error(t, err)
	})
}
