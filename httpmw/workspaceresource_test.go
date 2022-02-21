package httpmw_test

import (
	"testing"
)

func TestWorkspaceResource(t *testing.T) {
	t.Parallel()

	// setup := func(db database.Store) (*http.Request, database.WorkspaceResource) {
	// 	token := uuid.NewString()
	// 	r := httptest.NewRequest("GET", "/", nil)
	// 	r.AddCookie(&http.Cookie{
	// 		Name:  httpmw.AuthCookie,
	// 		Value: token,
	// 	})
	// 	resource, err := db.InsertWorkspaceResource(context.Background(), database.InsertWorkspaceResourceParams{
	// 		ID:                  uuid.New(),
	// 		CreatedAt:           database.Now(),
	// 		WorkspaceAgentToken: token,
	// 	})
	// 	require.NoError(t, err)
	// 	return r, resource
	// }

	// t.Run("None", func(t *testing.T) {
	// 	t.Parallel()
	// 	db := databasefake.New()
	// 	rtr := chi.NewRouter()
	// 	rtr.Use(
	// 		httpmw.ExtractWorkspaceResource()
	// 	)
	// 	rtr.Get("/", nil)
	// 	r, _ := setup(db)
	// 	rw := httptest.NewRecorder()
	// 	rtr.ServeHTTP(rw, r)

	// 	res := rw.Result()
	// 	defer res.Body.Close()
	// 	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	// })

	// t.Run("NotFound", func(t *testing.T) {
	// 	t.Parallel()
	// 	db := databasefake.New()
	// 	rtr := chi.NewRouter()
	// 	rtr.Use(
	// 		httpmw.ExtractAPIKey(db, nil),
	// 		httpmw.ExtractUserParam(db),
	// 		httpmw.ExtractWorkspaceParam(db),
	// 	)
	// 	rtr.Get("/", nil)
	// 	r, _ := setup(db)
	// 	chi.RouteContext(r.Context()).URLParams.Add("workspace", "frog")
	// 	rw := httptest.NewRecorder()
	// 	rtr.ServeHTTP(rw, r)

	// 	res := rw.Result()
	// 	defer res.Body.Close()
	// 	require.Equal(t, http.StatusNotFound, res.StatusCode)
	// })

	// t.Run("Found", func(t *testing.T) {
	// 	t.Parallel()
	// 	db := databasefake.New()
	// 	rtr := chi.NewRouter()
	// 	rtr.Use(
	// 		httpmw.ExtractAPIKey(db, nil),
	// 		httpmw.ExtractUserParam(db),
	// 		httpmw.ExtractWorkspaceParam(db),
	// 	)
	// 	rtr.Get("/", func(rw http.ResponseWriter, r *http.Request) {
	// 		_ = httpmw.WorkspaceParam(r)
	// 		rw.WriteHeader(http.StatusOK)
	// 	})
	// 	r, user := setup(db)
	// 	workspace, err := db.InsertWorkspace(context.Background(), database.InsertWorkspaceParams{
	// 		ID:      uuid.New(),
	// 		OwnerID: user.ID,
	// 		Name:    "hello",
	// 	})
	// 	require.NoError(t, err)
	// 	chi.RouteContext(r.Context()).URLParams.Add("workspace", workspace.Name)
	// 	rw := httptest.NewRecorder()
	// 	rtr.ServeHTTP(rw, r)

	// 	res := rw.Result()
	// 	defer res.Body.Close()
	// 	require.Equal(t, http.StatusOK, res.StatusCode)
	// })
}
