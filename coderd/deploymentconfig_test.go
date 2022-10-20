package coderd_test

// func TestDeploymentConfig(t *testing.T) {
// 	t.Parallel()
// 	hi := "hi"
// 	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitLong)
// 	defer cancel()
// 	vip := deployment.NewViper()
// 	cfg := deployment.Config(vip)
// 	// values should be returned
// 	cfg.AccessURL.Value = hi
// 	// values should not be returned
// 	cfg.OAuth2GithubClientSecret.Value = hi
// 	cfg.OIDCClientSecret.Value = hi
// 	cfg.PostgresURL.Value = hi
// 	cfg.SCIMAuthHeader.Value = hi

// 	client := coderdtest.New(t, &coderdtest.Options{
// 		DeploymentConfig: &cfg,
// 	})
// 	_ = coderdtest.CreateFirstUser(t, client)
// 	scrubbed, err := client.DeploymentConfig(ctx)
// 	require.NoError(t, err)
// 	// ensure normal values pass through
// 	require.EqualValues(t, hi, scrubbed.AccessURL.Value)
// 	// ensure secrets are removed
// 	require.Empty(t, scrubbed.OAuth2GithubClientSecret.Value)
// 	require.Empty(t, scrubbed.OIDCClientSecret.Value)
// 	require.Empty(t, scrubbed.PostgresURL.Value)
// 	require.Empty(t, scrubbed.SCIMAuthHeader.Value)
// }
