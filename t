[33mcommit 1072f967ed79c047aee0c1db3dd9d4038737d247[m[33m ([m[1;36mHEAD -> [m[1;32mbq/3516[m[33m, [m[1;31morigin/bq/3516[m[33m)[m
Merge: ba52fedf 8556bc96
Author: Bruno Quaresma <bruno@coder.com>
Date:   Tue Sep 6 14:20:52 2022 +0000

    Merge branch 'bq/3516' of github.com:coder/coder into bq/3516

[33mcommit ba52fedfdf3a5a9d74bc49962a0560ea885d4bfd[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Tue Sep 6 14:20:43 2022 +0000

    Add CODER_ENABLE_WILDCARD_APPS env var

[33mcommit 8e7f8bd86c81af10dd388ccdf2cb18cd7de151e5[m
Merge: a6f976e4 1b56a8cc
Author: Bruno Quaresma <bruno@coder.com>
Date:   Tue Sep 6 14:12:59 2022 +0000

    Merge branch 'main' of github.com:coder/coder into bq/3516

[33mcommit 1b56a8cccb46131b272fde411a39b7f48da71176[m
Author: Geoffrey Huntley <ghuntley@ghuntley.com>
Date:   Tue Sep 6 18:58:27 2022 +1000

    docs(readme): use /chat link in the README.md (#3868)

[33mcommit e3bbc77c35c7b99fb48fc514f261593fcc6fedab[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Mon Sep 5 18:02:17 2022 -0500

    chore: bump google.golang.org/api from 0.90.0 to 0.94.0 (#3882)
    
    Bumps [google.golang.org/api](https://github.com/googleapis/google-api-go-client) from 0.90.0 to 0.94.0.
    - [Release notes](https://github.com/googleapis/google-api-go-client/releases)
    - [Changelog](https://github.com/googleapis/google-api-go-client/blob/main/CHANGES.md)
    - [Commits](https://github.com/googleapis/google-api-go-client/compare/v0.90.0...v0.94.0)
    
    ---
    updated-dependencies:
    - dependency-name: google.golang.org/api
      dependency-type: direct:production
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit 1254e7a9026065183963cbf99be4a8284e28ba37[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Mon Sep 5 17:15:49 2022 -0500

    feat: Add speedtest command for tailnet (#3874)

[33mcommit 38825b9ab4fabdfdabd2f5a22bec9dcc1b2fc763[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Mon Sep 5 14:23:52 2022 -0500

    dogfood: keep image locally (#3878)
    
    Avoid delete conflicts

[33mcommit d6812e0be8d572363499905776774ed15d5f5436[m
Author: Geoffrey Huntley <ghuntley@ghuntley.com>
Date:   Tue Sep 6 04:38:29 2022 +1000

    housekeeping(codeowners): migrate to teams (#3867)

[33mcommit 2fa77a9bbd127297333f487fea1dfca00129ef04[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Mon Sep 5 10:43:24 2022 -0500

    fix: Run status callbacks async to solve tailnet race (#3866)

[33mcommit 3ca6f1fcd484d6041a5b4774621e919dce07c002[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Mon Sep 5 16:45:10 2022 +0300

    fix: Prevent nil pointer deref in reconnectingPTY (#3871)
    
    Related #3870

[33mcommit 1a5d3eace456760c05b0630e8232ccb8c5a87bb5[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Sun Sep 4 16:06:36 2022 -0500

    dogfood: dynamically pull image (#3864)
    
    Previously, the template would never pull new image updates.

[33mcommit 00f05e798be53ce6563b5539b4798b91c1706585[m
Author: Kyle Carberry <kyle@carberry.com>
Date:   Sun Sep 4 16:56:09 2022 +0000

    Fix `avatar_url` dump.sql

[33mcommit d8f953788054b302422fe5aa4f89671daefddd7a[m
Author: Kyle Carberry <kyle@carberry.com>
Date:   Sun Sep 4 16:55:25 2022 +0000

    Fix `avatar_url` database type

[33mcommit 05e2806ff3dbcc6021bf0561a97e6e14fa2d0ec7[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Sun Sep 4 11:44:27 2022 -0500

    feat: Add profile pictures to OAuth users (#3855)
    
    This supports GitHub and OIDC login for profile pictures!

[33mcommit 67c460537051baf1e738ba2b26a0e5646127d3a9[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Sun Sep 4 11:28:09 2022 -0500

    chore: Reduce test times (#3856)
    
    * chore: Reduce test times
    
    * Rename IncludeProvisionerD to IncludeProvisionerDaemon
    
    * Make  TestTemplateDAUs use Tailnet

[33mcommit 271d075667e1515dffea3d3b58c624ab0dc43075[m
Author: J Bruni <joaohbruni@yahoo.com.br>
Date:   Sun Sep 4 11:15:25 2022 -0300

    Update Coder contact at ADOPTERS.md (#3861)

[33mcommit 0a7fad674ae5f3423fb69916d8e54373e9ba2f9d[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Sat Sep 3 20:44:40 2022 -0500

    dogfood: remove github apt source (#3860)

[33mcommit 1b3e75c3abee0cf57e7af2355691049c7a232191[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Sat Sep 3 18:38:13 2022 -0500

    add watchexec to dogfood image (#3858)
    
    * add watchexec to dogfood image
    
    This comes in handy quite frequently.
    
    * Fix dogfood image

[33mcommit aae57476f18ae1e957cb3d6302d1e4a42cfd683b[m
Author: Geoffrey Huntley <ghuntley@ghuntley.com>
Date:   Sat Sep 3 16:18:04 2022 +1000

    docs(adopters): add ADOPTERS.md (#3825)

[33mcommit 037258638265ac3f1297fea3dd02b123ce92a0a5[m
Author: Geoffrey Huntley <ghuntley@ghuntley.com>
Date:   Sat Sep 3 16:16:57 2022 +1000

    housekeeping(discord): use /chat instead of the discord.gg link (#3826)

[33mcommit a24f26c13773eef3b209269e69f3e211859077bb[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Sep 2 18:47:25 2022 -0500

    fix: Allow disabling built-in DERP server (#3852)

[33mcommit 4f4d470c7ccb30234e9aebae2ebc8df9a28b13c1[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Sep 2 18:26:01 2022 -0500

    feat: Add wireguard to port-forward (#3851)
    
    This allows replacement of the WebRTC networking!

[33mcommit a09ffd6c0dc969b8a1fa57a8c201dc79118fa451[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Fri Sep 2 17:48:40 2022 -0500

    feat: show better error on invalid template upload (#3847)
    
    * feat: show better error on invalid template upload
    
    * Fix tests

[33mcommit ac500707138ad041deb217effc2580614aaa4419[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Sep 2 17:05:27 2022 -0500

    fix: Add omitempty for proper latency type (#3850)
    
    This was causing an error on the frontend, because this value can be nil!

[33mcommit 2e1db6cc63fb1269547b1aed407fe9f44d3535ac[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Sep 2 15:09:05 2022 -0500

    feat: Add latency indicator to the UI (#3846)
    
    With Tailscale, we now get latency of all regions.

[33mcommit e490bdd531be5cdfad0e72acc184e4edddd17b03[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Sep 2 14:46:58 2022 -0500

    revert: Makefile buff-ification (#3700) (#3848)
    
    This caused the following issues:
    - Slim binaries weren't being updated.
    - The coder.tar.ztd was misplaced.
    - There is no coder.sha1 file with proper filenames.
    
    This should be reintroduced in a future change with those fixes.

[33mcommit d350d9033ce5c4a19ec4a898ea389b45565fc986[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 16:32:28 2022 -0300

    refactor: Remove extra line from table bottom (#3831)

[33mcommit ff0aa8d742c0afc9ce6a0f6a5c115af85f49ab7e[m
Author: Colin Adler <colin1adler@gmail.com>
Date:   Fri Sep 2 13:04:29 2022 -0500

    feat: add unique ids to all HTTP requests (#3845)

[33mcommit de219d966d46783bdb515b7d6989e2737669dc83[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Sep 2 11:58:15 2022 -0500

    fix: Run Tailnet SSH connections in a goroutine (#3838)
    
    This was causing SSH connections in parallel to fail 🤦!

[33mcommit 3be7bb58b4eece7ce7515986ce2189674f855c9e[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Fri Sep 2 16:58:04 2022 +0000

    chore: bump @storybook/addon-essentials from 6.4.22 to 6.5.10 in /site (#3827)
    
    Bumps [@storybook/addon-essentials](https://github.com/storybookjs/storybook/tree/HEAD/addons/essentials) from 6.4.22 to 6.5.10.
    - [Release notes](https://github.com/storybookjs/storybook/releases)
    - [Changelog](https://github.com/storybookjs/storybook/blob/v6.5.10/CHANGELOG.md)
    - [Commits](https://github.com/storybookjs/storybook/commits/v6.5.10/addons/essentials)
    
    ---
    updated-dependencies:
    - dependency-name: "@storybook/addon-essentials"
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit 6fe63ed358449e849f445b6c36ad7358eb476c98[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 13:53:46 2022 -0300

    refactor: Keep focused style when input is hovered (#3832)

[33mcommit 56186402273e263735fcd77074a7db1d873466eb[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 13:49:41 2022 -0300

    refactor: Remove duplicated title (#3829)

[33mcommit 55c13c8ff98ea5abf7286486214e6811bb92eaa2[m
Author: Colin Adler <colin1adler@gmail.com>
Date:   Fri Sep 2 11:42:28 2022 -0500

    chore: fully implement enterprise audit pkg (#3821)

[33mcommit fefdff49461e63acad9775af9b62dead8a6bb144[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Sat Sep 3 02:38:33 2022 +1000

    fix: install goimports in deploy build (#3841)

[33mcommit e6699d25ca5dbdfba3aa21c4f5d7305b589c597b[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Sat Sep 3 02:16:19 2022 +1000

    fix: fix CI calling script/version.sh instead of scripts (#3839)

[33mcommit 8c70b6c360484b9dca129a90e051eae35b98f8ae[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 13:04:08 2022 -0300

    refactor: Update table cell colors to match the ones in the Workspace (#3830)
    
    page

[33mcommit 21ae4112370ddf205f02d4d8472f42e231848459[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 13:03:59 2022 -0300

    refactor: Fix README spacing (#3833)

[33mcommit b9e5cc97a112a72727b92331484dfd3945f101db[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 13:03:36 2022 -0300

    refactor: Make user columns consistent (#3834)

[33mcommit f1976a086f65036b4efc4f6d41e8879ec6aa6c56[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Fri Sep 2 10:25:26 2022 -0500

    chore: bump webpack-bundle-analyzer from 4.5.0 to 4.6.1 in /site (#3818)
    
    Bumps [webpack-bundle-analyzer](https://github.com/webpack-contrib/webpack-bundle-analyzer) from 4.5.0 to 4.6.1.
    - [Release notes](https://github.com/webpack-contrib/webpack-bundle-analyzer/releases)
    - [Changelog](https://github.com/webpack-contrib/webpack-bundle-analyzer/blob/master/CHANGELOG.md)
    - [Commits](https://github.com/webpack-contrib/webpack-bundle-analyzer/compare/v4.5.0...v4.6.1)
    
    ---
    updated-dependencies:
    - dependency-name: webpack-bundle-analyzer
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit e20ff62c9f35af22290ed7bad314a442dc6de5eb[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Fri Sep 2 13:11:20 2022 +0000

    chore: bump xstate from 4.32.1 to 4.33.5 in /site (#3817)
    
    Bumps [xstate](https://github.com/statelyai/xstate) from 4.32.1 to 4.33.5.
    - [Release notes](https://github.com/statelyai/xstate/releases)
    - [Commits](https://github.com/statelyai/xstate/compare/xstate@4.32.1...xstate@4.33.5)
    
    ---
    updated-dependencies:
    - dependency-name: xstate
      dependency-type: direct:production
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit 8556bc96a03c3c0dae12a9631c1f5c82210b8397[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Sep 2 10:10:22 2022 -0300

    Update site/src/components/PortForwardButton/PortForwardButton.tsx
    
    Co-authored-by: Presley Pizzo <1290996+presleyp@users.noreply.github.com>

[33mcommit afd6834ff7a5a4b70430d5c87e8240a2ce4d9397[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Fri Sep 2 10:08:26 2022 -0300

    chore: bump @typescript-eslint/eslint-plugin in /site (#3804)
    
    Bumps [@typescript-eslint/eslint-plugin](https://github.com/typescript-eslint/typescript-eslint/tree/HEAD/packages/eslint-plugin) from 5.31.0 to 5.36.1.
    - [Release notes](https://github.com/typescript-eslint/typescript-eslint/releases)
    - [Changelog](https://github.com/typescript-eslint/typescript-eslint/blob/main/packages/eslint-plugin/CHANGELOG.md)
    - [Commits](https://github.com/typescript-eslint/typescript-eslint/commits/v5.36.1/packages/eslint-plugin)
    
    ---
    updated-dependencies:
    - dependency-name: "@typescript-eslint/eslint-plugin"
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit e1a4f3a16b674adc31533ef713015a92a9ecd329[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Fri Sep 2 22:58:23 2022 +1000

    Makefile buff-ification (#3700)
    
    Remove old go_build_matrix and go_build_slim scripts in favor of full makefile-ification.

[33mcommit 46bf265e9b8d99fdb96e24b8c4c2422263fd1725[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Fri Sep 2 21:01:30 2022 +1000

    fix: prevent running helm chart without valid tag (#3770)
    
    Co-authored-by: Eric Paulsen <ericpaulsen@coder.com>

[33mcommit 4c180342604eaf2e0cef981e89fd4769b468e727[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Fri Sep 2 13:24:47 2022 +0300

    fix: Prevent autobuild executor from slowing down API requests (#3726)
    
    With just a few workspaces, the autobuild executor can slow down API
    requests every time it runs. This is because we started a long running
    transaction and checked all eligible (for autostart) workspaces inside
    that transaction. PostgreSQL doesn't know if we're modifying rows and as
    such is locking the tables for read operations.
    
    This commit changes the behavior so each workspace is checked in its own
    transaction reducing the time the table/rows needs to stay locked.
    
    For now concurrency has been arbitrarily limited to 10 workspaces at a
    time, this could be made configurable or adjusted as the need arises.

[33mcommit 3f73243b37723bde5f8c2f5c97edf66659433520[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Thu Sep 1 23:03:02 2022 -0500

    feat: improve formatting of last used (#3824)

[33mcommit 2d347657dc4540a53d05cdb2643f72caf39638eb[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Thu Sep 1 21:29:57 2022 -0500

    site: correct documentation on gitsshkey (#3690)
    
    * site: correct documentation on gitsshkey
    
    Co-authored-by: Presley Pizzo <1290996+presleyp@users.noreply.github.com>

[33mcommit 3c91b92930923d167b90410fbd5e177d313a7cca[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Thu Sep 1 18:16:20 2022 -0700

    docs: add comment to ResourceAvatar (#3822)

[33mcommit 04b03792cbf8f31551b59e9c1947a8d85d660133[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Thu Sep 1 19:08:51 2022 -0500

    feat: add last used to Workspaces page (#3816)

[33mcommit 80e9f24ac73b705c8b20770c4c7105d326d938a0[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Thu Sep 1 19:58:43 2022 -0400

    feat: add loaders to ssh and terminal buttons (#3820)

[33mcommit be273a20a7699c13a7ba488d7cfbe1efd0a224ff[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Thu Sep 1 17:02:05 2022 -0500

    fix: Update Tailscale to add HTTP(s) latency reporting (#3819)
    
    This was broken in Tailscale, and I'll be sending an upstream PR
    to resolve it. See: https://github.com/coder/tailscale/commit/2c5af585574d4e1432f0d5dc9d02c63db3f497b0

[33mcommit 081259314bcdd3f9f64b749c4a6ae56a408f2aa3[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Thu Sep 1 21:25:53 2022 +0000

    chore: bump cron-parser from 4.5.0 to 4.6.0 in /site (#3809)
    
    Bumps [cron-parser](https://github.com/harrisiirak/cron-parser) from 4.5.0 to 4.6.0.
    - [Release notes](https://github.com/harrisiirak/cron-parser/releases)
    - [Commits](https://github.com/harrisiirak/cron-parser/compare/4.5.0...4.6.0)
    
    ---
    updated-dependencies:
    - dependency-name: cron-parser
      dependency-type: direct:production
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit ff026d48903be29669f2c8bbef2fbdd69e4b69a4[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Thu Sep 1 14:20:08 2022 -0700

    chore: bump eslint-plugin-react from 7.30.1 to 7.31.1 in /site (#3806)
    
    Bumps [eslint-plugin-react](https://github.com/jsx-eslint/eslint-plugin-react) from 7.30.1 to 7.31.1.
    - [Release notes](https://github.com/jsx-eslint/eslint-plugin-react/releases)
    - [Changelog](https://github.com/jsx-eslint/eslint-plugin-react/blob/master/CHANGELOG.md)
    - [Commits](https://github.com/jsx-eslint/eslint-plugin-react/compare/v7.30.1...v7.31.1)
    
    ---
    updated-dependencies:
    - dependency-name: eslint-plugin-react
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit cde036c1ab590d58ac4cd8b26235351b9c74d8f7[m[33m ([m[1;33mtag: v0.8.11[m[33m)[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Thu Sep 1 15:10:53 2022 -0500

    fix: Update to Go 1.19 for releases (#3814)

[33mcommit 30f8fd9b952f3788092efb88f74bec66b679e559[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Thu Sep 1 14:58:23 2022 -0500

    Daily Active User Metrics (#3735)
    
    * agent: add StatsReporter
    
    * Stabilize protoc

[33mcommit a6f976e48f2f96d6b1f21c04da340f38487137dc[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Thu Sep 1 18:51:04 2022 +0000

    feat: Add portforward to the UI

[33mcommit e0cb52ceeaf01ffbf05852a14af339a9a4c04980[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Thu Sep 1 13:43:52 2022 -0500

    fix: Use an unnamed region instead of erroring for DERP (#3810)

[33mcommit 5f0b13795aac5486c7980b5ca8c53c80498bac08[m
Author: Presley Pizzo <1290996+presleyp@users.noreply.github.com>
Date:   Thu Sep 1 14:28:18 2022 -0400

    feat: make scrollbars match color scheme (#3807)

[33mcommit 1efcd33d6352dc659aa31cd72911e34dd9821da7[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Thu Sep 1 14:26:45 2022 -0400

    chore: bump jest-runner-eslint from 1.0.0 to 1.1.0 in /site (#3799)
    
    Bumps [jest-runner-eslint](https://github.com/jest-community/jest-runner-eslint) from 1.0.0 to 1.1.0.
    - [Release notes](https://github.com/jest-community/jest-runner-eslint/releases)
    - [Changelog](https://github.com/jest-community/jest-runner-eslint/blob/main/CHANGELOG.md)
    - [Commits](https://github.com/jest-community/jest-runner-eslint/compare/v1.0.0...v1.1.0)
    
    ---
    updated-dependencies:
    - dependency-name: jest-runner-eslint
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit 6d95145d3b16fe22f6d70c2462002195254a5a44[m
Author: Presley Pizzo <1290996+presleyp@users.noreply.github.com>
Date:   Thu Sep 1 14:24:14 2022 -0400

    Feat: delete template button (#3781)
    
    * Add api call
    
    * Extract DropDownButton
    
    * Start adding DropdownButton to Template page
    
    * Move stories to dropdown button
    
    * Format
    
    * Update xservice to delete
    
    * Deletion flow
    
    * Format
    
    * Move ErrorSummary for consistency
    
    * RBAC (unfinished) and style tweak
    
    * Format
    
    * Test rbac
    
    * Format
    
    * Move ErrorSummary under PageHeader in workspace and template
    
    * Format
    
    * Replace hook with onBlur
    
    * Make style arg optional
    
    * Format

[33mcommit 6826b976d760439188c11b242fc46053e4b3e799[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Thu Sep 1 11:41:47 2022 -0500

    fix: Add latency-check for DERP over HTTP(s) (#3788)
    
    * fix: Add latency-check for DERP over HTTP(s)
    
    This fixes scenarios where latency wasn't being reported if
    a connection had UDP entirely blocked.
    
    * Add inactivity ping
    
    * Improve coordinator error reporting consistency

[33mcommit f4c8bfdc18b624df96e16408f47f4d3f0da36843[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Thu Sep 1 16:26:50 2022 +0000

    chore: bump webpack-dev-server from 4.9.3 to 4.10.1 in /site (#3801)
    
    Bumps [webpack-dev-server](https://github.com/webpack/webpack-dev-server) from 4.9.3 to 4.10.1.
    - [Release notes](https://github.com/webpack/webpack-dev-server/releases)
    - [Changelog](https://github.com/webpack/webpack-dev-server/blob/master/CHANGELOG.md)
    - [Commits](https://github.com/webpack/webpack-dev-server/compare/v4.9.3...v4.10.1)
    
    ---
    updated-dependencies:
    - dependency-name: webpack-dev-server
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit 5b9573d7c13a14de76c99d934d1f542909813d6f[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Thu Sep 1 09:15:42 2022 -0700

    chore: bump just-debounce-it from 3.0.1 to 3.1.1 in /site (#3800)
    
    Bumps [just-debounce-it](https://github.com/angus-c/just) from 3.0.1 to 3.1.1.
    - [Release notes](https://github.com/angus-c/just/releases)
    - [Commits](https://github.com/angus-c/just/compare/just-debounce-it@3.0.1...just-pick@3.1.1)
    
    ---
    updated-dependencies:
    - dependency-name: just-debounce-it
      dependency-type: direct:production
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit b57b8b887d22d1523e351bc99744de44daeeaf82[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Thu Sep 1 09:14:57 2022 -0700

    chore: bump jest-websocket-mock from 2.3.0 to 2.4.0 in /site (#3797)
    
    Bumps [jest-websocket-mock](https://github.com/romgain/jest-websocket-mock) from 2.3.0 to 2.4.0.
    - [Release notes](https://github.com/romgain/jest-websocket-mock/releases)
    - [Commits](https://github.com/romgain/jest-websocket-mock/compare/v2.3.0...v2.4.0)
    
    ---
    updated-dependencies:
    - dependency-name: jest-websocket-mock
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit f4a78c976f59a1bcf118252d6163aeabb55934bf[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Thu Sep 1 13:24:08 2022 +0300

    docs: Update `direnv` docs for Nix and remove `.envrc` (#3790)

[33mcommit 567e7506599a1a90123a09dda4dfc9dfc2d23509[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Wed Aug 31 21:21:21 2022 -0500

    fix: Prepend STUN nodes for DERP (#3787)
    
    This makes Tailscale prefer STUN over DERP when possible.

[33mcommit 9bd83e5ec76909de7bc15fba84d5d71b6597fdca[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Wed Aug 31 20:09:44 2022 -0500

    feat: Add Tailscale networking (#3505)
    
    * fix: Add coder user to docker group on installation
    
    This makes for a simpler setup, and reduces the likelihood
    a user runs into a strange issue.
    
    * Add wgnet
    
    * Add ping
    
    * Add listening
    
    * Finish refactor to make this work
    
    * Add interface for swapping
    
    * Fix conncache with interface
    
    * chore: update gvisor
    
    * fix tailscale types
    
    * linting
    
    * more linting
    
    * Add coordinator
    
    * Add coordinator tests
    
    * Fix coordination
    
    * It compiles!
    
    * Move all connection negotiation in-memory
    
    * Migrate coordinator to use net.conn
    
    * Add closed func
    
    * Fix close listener func
    
    * Make reconnecting PTY work
    
    * Fix reconnecting PTY
    
    * Update CI to Go 1.19
    
    * Add CLI flags for DERP mapping
    
    * Fix Tailnet test
    
    * Rename ConnCoordinator to TailnetCoordinator
    
    * Remove print statement from workspace agent test
    
    * Refactor wsconncache to use tailnet
    
    * Remove STUN from unit tests
    
    * Add migrate back to dump
    
    * chore: Upgrade to Go 1.19
    
    This is required as part of #3505.
    
    * Fix reconnecting PTY tests
    
    * fix: update wireguard-go to fix devtunnel
    
    * fix migration numbers
    
    * linting
    
    * Return early for status if endpoints are empty
    
    * Update cli/server.go
    
    Co-authored-by: Colin Adler <colin1adler@gmail.com>
    
    * Update cli/server.go
    
    Co-authored-by: Colin Adler <colin1adler@gmail.com>
    
    * Fix frontend entites
    
    * Fix agent bicopy
    
    * Fix race condition for the last node
    
    * Fix down migration
    
    * Fix connection RBAC
    
    * Fix migration numbers
    
    * Fix forwarding TCP to a local port
    
    * Implement ping for tailnet
    
    * Rename to ForceHTTP
    
    * Add external derpmapping
    
    * Expose DERP region names to the API
    
    * Add global option to enable Tailscale networking for web
    
    * Mark DERP flags hidden while testing
    
    * Update DERP map on reconnect
    
    * Add close func to workspace agents
    
    * Fix race condition in upstream dependency
    
    * Fix feature columns race condition
    
    Co-authored-by: Colin Adler <colin1adler@gmail.com>

[33mcommit 00da01fdf7021515af9b755248f96694e19ec1f6[m
Author: Colin Adler <colin1adler@gmail.com>
Date:   Wed Aug 31 16:12:54 2022 -0500

    chore: rearrange audit logging code into enterprise folder (#3741)

[33mcommit 9583e16a059b31b5eccbc1dd9e9528f375fd9acc[m
Author: Mickael <24225884+mickaelicoptere@users.noreply.github.com>
Date:   Wed Aug 31 22:40:41 2022 +0200

    Update community-templates.md (#3785)
    
    added kubernetes dind template

[33mcommit 5362f4636ef9b588c71d99d2bee5bd942207ddce[m[33m ([m[1;31morigin/3767-fix-types-generated-for-workspaceresource-type-field[m[33m)[m
Author: Cian Johnston <cian@coder.com>
Date:   Wed Aug 31 16:33:50 2022 +0100

    feat: show agent version in UI and CLI (#3709)
    
    This commit adds the ability for agents to set their version upon start.
    This is then reported in the UI and CLI.

[33mcommit aa9a1c3f56de77a99717c075ced1e1967f4be3d2[m
Author: Steven Masley <Emyrk@users.noreply.github.com>
Date:   Wed Aug 31 11:26:36 2022 -0400

    fix: Prevent suspending owners (#3757)

[33mcommit e6802f0a5653986a223fd52ed02feddc8ecdbbef[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Wed Aug 31 07:44:20 2022 -0700

    refactor: use WidgetsIcon for null resources (#3754)
    
    * refactor: replace HelpIcon w/WidgetsIcon
    
    Based on user feedback, we believe the `WidgetsIcon` will cause less
    confusion.
    
    * fixup
    
    * refactor: clean up types in ResourceAvatar.tsx
    
    Before, we were using `string` for `type` in `ResourceAvatar`. This
    meant it wasn't tied to the types generated from the backend.
    
    Now it imports `WorkspaceResource` so that there is a single source of
    truth and they always stay in sync.

[33mcommit 774d7588ddf4d885fd40af02faa17e34eb6ac391[m
Author: Muhammad Atif Ali <matifali@live.com>
Date:   Wed Aug 31 15:04:16 2022 +0300

    docs: Update community-templates.md (#3778)
    
    Added docker based deep learning and matlab coder-templates

[33mcommit 126d71f41d4f8be43a2f64b7147070ffa015c78b[m
Author: Michael Eanes <97917268+maeanes@users.noreply.github.com>
Date:   Tue Aug 30 23:23:56 2022 -0400

    Remove alpha warning from about (#3774)
    
    The doc was outdated; I don't think the software is alpha anymore.

[33mcommit 6644e951d8417f4a3a2f7e81a1db6afdd202b256[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Tue Aug 30 19:17:57 2022 -0500

    fix: Scope error to test functions to fix TestFeaturesService race (#3765)
    
    Fixes #3747.

[33mcommit 02c0100d4d0220d0f38d373025647b904f310ab8[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Tue Aug 30 19:56:36 2022 -0300

    fix: Use a select when parameter input has many options (#3762)

[33mcommit 01a06e1213ef5e0c3a705d27a19551f138d275de[m[33m ([m[1;33mtag: v0.8.10[m[33m)[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Tue Aug 30 15:18:10 2022 -0400

    feat: Add dedicated labels to agent status and OS (#3759)

[33mcommit a410ac42f5ed96dd330155e23c5f9ccb5028e1de[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Tue Aug 30 14:00:23 2022 -0500

    fix: Use first user for telemetry email (#3761)
    
    This was causing other users email to be sent, which isn't desired.

[33mcommit f037aad456b7134215e9dcf2201e4b0dd72bdd30[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Tue Aug 30 15:48:03 2022 -0300

    fix: Accepts empty string for the icon prop to remove  it (#3760)

[33mcommit 1dc0485027d1c8d39f662c1842fbd18989a4e4fe[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Tue Aug 30 21:08:20 2022 +0300

    fix: Use smarter quoting for ProxyCommand in config-ssh (#3755)
    
    * fix: Use smarter quoting for ProxyCommand in config-ssh
    
    This change takes better into account how OpenSSH executes
    `ProxyCommand`s and applies quoting accordingly.
    
    This supercedes #3664, which was reverted.
    
    Fixes #2853
    
    * fix: Ensure `~/.ssh` directory exists

[33mcommit 0708e37a38ce1fbc74803d3ba44354149db294ad[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Tue Aug 30 14:27:33 2022 -0300

    feat: Sort templates by workspaces count (#3734)

[33mcommit 190310464d715b16f1568bf975e823e4413ad022[m
Author: Muhammad Atif Ali <matifali@live.com>
Date:   Tue Aug 30 19:18:04 2022 +0300

    Update `username` in connecting to a workspace documenation (using JetBrains Gateway) (#3746)
    
    if someone is not using coder-provided templates, they might not have coder as a user name.

[33mcommit 8a60ee03917e9dc4133a30b04311704fe70fad5f[m
Author: Eric Paulsen <ericpaulsen@coder.com>
Date:   Tue Aug 30 10:55:40 2022 -0500

    add: code-server to template examples (#3739)
    
    * add: code-server to template examples
    
    * add: code-server to gcp templates
    
    * add: code-server to gcp-linux template
    
    * update: READMEs
    
    * update: boot disk version
    
    * update: google provider version

[33mcommit 20086c1e77be7acbcc341c0f30117aac396f4f50[m
Author: Geoffrey Huntley <ghuntley@ghuntley.com>
Date:   Tue Aug 30 12:33:11 2022 +1000

    feat(devenv): use direnv to invoke nix-shell (#3745)

[33mcommit c4a9be9c410d6d0ec260009068e8f4ca4c81fbf3[m
Author: Eric Paulsen <ericpaulsen@coder.com>
Date:   Mon Aug 29 19:12:26 2022 -0500

    update: google provider to latest (#3743)
    
    * update: google provider to latest
    
    * rm: code-server

[33mcommit cc346afce6193c312e9f7f5fa95a263fd6451ef5[m
Author: Spike Curtis <spike@coder.com>
Date:   Mon Aug 29 16:45:40 2022 -0700

    Use licenses to populate the Entitlements API (#3715)
    
    * Use licenses for entitlements API
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Tests for entitlements API
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Add commentary about FeatureService
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Lint
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Quiet down the logs
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Tell revive it's ok
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 05f932b37e10810cf0128d7bd49783ef9346955a[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Mon Aug 29 15:05:08 2022 -0700

    refactor(scripts): remove -P from ln calls (#3740)

[33mcommit 053fe6ff61546cbb07d2272102aca456b8830963[m
Author: Jon Ayers <jon@coder.com>
Date:   Mon Aug 29 17:00:52 2022 -0500

    feat: add panic recovery middleware (#3687)

[33mcommit 3cf17d34e7cfab3201da05b8d07516c136bc64ab[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Mon Aug 29 16:57:54 2022 -0300

    refactor: Redesign auth cli page and add workspaces link (#3737)

[33mcommit 779c446a6efd9865e3383c72b972f00d3b42e8cb[m
Author: Spike Curtis <spike@coder.com>
Date:   Mon Aug 29 11:30:06 2022 -0700

    cli prints license warnings (#3716)
    
    * cli prints license warnings
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Satisfy the linter
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 62f686c003e290f89d46a04e53230aa904bff41e[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Mon Aug 29 14:49:04 2022 -0300

    fix: Templates table columns width (#3731)

[33mcommit 6285d65b6a752896870ebdd5abae3b1afd8ac529[m[33m ([m[1;33mtag: v0.8.9[m[33m)[m
Author: Colin Adler <colin1adler@gmail.com>
Date:   Mon Aug 29 12:07:49 2022 -0500

    fix: remove `(http.Server).ReadHeaderTimeout` (#3730)
    
    * fix: remove `(http.Server).ReadHeaderTimeout`
    
    Fixes https://github.com/coder/coder/issues/3710. It caused some race
    condition for websockets where the server sent the first message.
    
    * comment why disabled

[33mcommit 611ca55458013617d5d80060409fabe78475d71b[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Mon Aug 29 11:32:57 2022 -0500

    fix: Use "data" scheme when creating parameters from the site (#3732)
    
    Fixes #3691.

[33mcommit 34d902ebf19ea294db0b906e28c22e89c698b67a[m
Author: Steven Masley <Emyrk@users.noreply.github.com>
Date:   Mon Aug 29 08:56:52 2022 -0400

    fix: Fix properly selecting workspace apps by agent (#3684)

[33mcommit dc9b4155e0028433f37dbe2db86e38917b1e81ad[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Mon Aug 29 14:56:51 2022 +0300

    feat: Generate DB unique constraints as enums (#3701)
    
    * feat: Generate DB unique constraints as enums
    
    This fixes a TODO from #3409.

[33mcommit f4c5020f63abff6c826e99bffcd51b50cb0b1a90[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Mon Aug 29 14:37:18 2022 +0300

    fix: Print postgres-builtin-url to stdout without formatting (#3727)
    
    This allows use-cases like `eval $(coder server postgres-builtin-url)`.

[33mcommit b9b9c2fb9f66d4385267f97d20b690c0856572a6[m[33m ([m[1;33mtag: v0.8.8[m[33m)[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Sun Aug 28 01:03:10 2022 +1000

    fix: mount TLS secret in helm chart (#3717)

[33mcommit ccabec6dd187b1551df9d516a93cabbf8dc48f16[m[33m ([m[1;33mtag: v0.8.7[m[33m)[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Fri Aug 26 11:18:42 2022 -0400

    fi stop tracing 4xx http status codes as errors (#3707)

[33mcommit 23f61fce2a38b2e28962a87ce5dcd4fc300c5858[m
Author: Spike Curtis <spike@coder.com>
Date:   Fri Aug 26 08:15:46 2022 -0700

    CLI: coder licensese delete (#3699)
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 98a6958f1059e39a2a398ff77a199cc66b721689[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Fri Aug 26 17:52:25 2022 +0300

    Revert "fix: Avoid double escaping of ProxyCommand on Windows (#3664)" (#3704)
    
    This reverts commit 123fe0131eacef645c64c60226a64c097abc5906.

[33mcommit 6a00baf235f3583eb8c55060793947d65fcd1b58[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Fri Aug 26 17:38:40 2022 +0300

    fix: Transform branch name to valid Docker tag for dogfood (#3703)

[33mcommit c8f8c95f6ac23f2f21ede41149eb90cd68422685[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Fri Aug 26 12:28:38 2022 +0300

    feat: Add support for renaming workspaces (#3409)
    
    * feat: Implement workspace renaming
    
    * feat: Add hidden rename command (and data loss warning)
    
    * feat: Implement database.IsUniqueViolation

[33mcommit 623fc5baace9cfbb13c8c763bb2d46e0e9f16134[m
Author: Presley Pizzo <1290996+presleyp@users.noreply.github.com>
Date:   Thu Aug 25 19:20:31 2022 -0400

    feat: condition Audit log on licensing (#3685)
    
    * Update XService
    
    * Add simple wrapper
    
    * Add selector
    
    * Condition page
    
    * Condition link
    
    * Format and lint
    
    * Integration test
    
    * Add username to api call
    
    * Format
    
    * Format
    
    * Fix link name
    
    * Upgrade xstate/react to fix crashing tests
    
    * Fix tests
    
    * Format
    
    * Abstract strings
    
    * Debug test
    
    * Increase timeout
    
    * Add comments and try shorter timeout
    
    * Use PropsWithChildren
    
    * Undo PropsWithChildren, try lower timeout
    
    * Format, lower timeout

[33mcommit ca3811499ec9e997d40cf177cd460fe9432912de[m
Author: Spike Curtis <spike@coder.com>
Date:   Thu Aug 25 14:04:31 2022 -0700

    DELETE license API endpoint (#3697)
    
    * DELETE license API endpoint
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Fix new lint stuff
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 14a9576b775395d73d8a9e5ca874ff21922373e7[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Fri Aug 26 05:32:35 2022 +1000

    Auto import kubernetes template in Helm charts (#3550)

[33mcommit 94e96fa40b676e11d44a758e0e7ec5d8ceeb3e55[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Thu Aug 25 11:20:24 2022 -0700

    chore: enable react/no-array-index-key eslint (#3696)
    
    * chore: enable react/no-array-index-key eslint
    
    * fix: add missing key to ResourcesTable

[33mcommit 8a446837d430d0fc9bc9800a2d9441d1f5a3d1d2[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Fri Aug 26 04:03:27 2022 +1000

    chore: remove exa -> ls and bat -> cat replacements from dogfood img (#3695)

[33mcommit 7a77e55bd442406a6d902bd42bc8b0118ec68a3b[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Thu Aug 25 12:34:37 2022 -0400

    fix: match term color (#3694)

[33mcommit b412cc1a4bf7b037af03f8831d46dcb56f5b2683[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Thu Aug 25 12:24:43 2022 -0400

    fix: use correct response writer for tracing middle (#3693)

[33mcommit 78a24941fe5a70752d538eab30fa875ceb943f61[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Thu Aug 25 19:10:42 2022 +0300

    feat: Add `codersdk.NullTime`, change workspace build deadline (#3552)
    
    Fixes #2015
    
    Co-authored-by: Joe Previte <jjprevite@gmail.com>

[33mcommit a21a6d2f4aa1f3c4b1e72eecd97db3fd369d2e5d[m
Author: Roman Zubov <zuroin@gmail.com>
Date:   Thu Aug 25 18:26:04 2022 +0300

    docs: replaced manual up next blocks with doc tag in workspaces.md (#3023)
    
    * docs: replaced manual up next blocks with doc tag in workspaces.md
    
    * replaced up next blocks with <doc page=""> tags
    
    * revert back to markdown
    
    now that we updated how these links work, we can have them as markdown on github and as cards on the docs website.
    
    Co-authored-by: Anton Korzhuk <antonkorzhuk@gmail.com>

[33mcommit 4de1fc833993aa213af564d159b4b142c5e6d5a3[m
Author: Spike Curtis <spike@coder.com>
Date:   Thu Aug 25 08:24:39 2022 -0700

    CLI: coder licenses list (#3686)
    
    * Check GET license calls authz
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * CLI: coder licenses list
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit a05fad4efd3af84b05ce12c634fd28a15f30932b[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Thu Aug 25 09:37:59 2022 -0400

    fix: stop tracing static file server (#3683)

[33mcommit 6e496077ae89ee3b716d0498954eb1265fb55766[m
Author: Steven Masley <Emyrk@users.noreply.github.com>
Date:   Wed Aug 24 17:43:41 2022 -0400

    feat: Support search query and --me in workspace list (#3667)

[33mcommit cf0d2c9bbc2a7dc5758cb230aec9c72aa21cd9e9[m
Author: Kira Pilot <kira@coder.com>
Date:   Wed Aug 24 17:28:02 2022 -0400

    added react-i18next to FE (#3682)
    
    * added react-i18next
    
    * fixing typo
    
    * snake case to camel case
    
    * typo
    
    * clearer error in catch block

[33mcommit e6b6b7f6102deb2c75d42c50914f536c179b5752[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Wed Aug 24 13:45:03 2022 -0700

    chore: upload playwright videos on failure (#3677)

[33mcommit 0b53b06fc63a135d328d78eed3342b53acf89805[m
Author: Steven Masley <Emyrk@users.noreply.github.com>
Date:   Wed Aug 24 15:58:57 2022 -0400

    chore: Make member role struct match site roles (#3671)

[33mcommit 076c4a0aa8b2a4fa9c8d0c56197302d2e73a617a[m
Author: Spike Curtis <spike@coder.com>
Date:   Wed Aug 24 12:25:37 2022 -0700

    Fix authz test for GET licenses (#3681)
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 9e35793b431b0b0a4bd25eef77989ea18ec7d8de[m
Author: Spike Curtis <spike@coder.com>
Date:   Wed Aug 24 12:05:46 2022 -0700

    Enterprise rbac testing (#3653)
    
    * WIP refactor Auth tests to allow enterprise
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * enterprise RBAC testing
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Fix import ordering
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 254e91a08f74bc875134ab6c37d02fede1331364[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Wed Aug 24 12:02:12 2022 -0700

    Update stale.yaml (#3674)
    
    - remove close-issue-reason (only valid in 5.1.0)
    - add days-before-issue-stale 30

[33mcommit 5d7c4092ac38ab984aaca33a753e1488f3cdeb58[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Wed Aug 24 14:57:31 2022 -0400

    fix: end long lived connection traces (#3679)

[33mcommit c9bce19d88e3f46ec5d29bab7ca02fb2a46684d7[m
Author: Spike Curtis <spike@coder.com>
Date:   Wed Aug 24 11:44:22 2022 -0700

    GET license endpoint (#3651)
    
    * GET license endpoint
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * SDK GetLicenses -> Licenses
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit da5487495858b8f89ba75bc5d55b6442251cc713[m
Author: Kira Pilot <kira@coder.com>
Date:   Wed Aug 24 14:10:41 2022 -0400

    fixed users test (#3676)

[33mcommit 57c202d112180fa26d10d414a78857a737c7d42a[m
Author: Kira Pilot <kira@coder.com>
Date:   Wed Aug 24 14:07:56 2022 -0400

    Template settings fixes/kira pilot (#3668)
    
    * using hours instead of seconds
    
    * checking out
    
    * added ttl tests
    
    * added description validation  and tests
    
    * added some helper text
    
    * fix typing
    
    * Update site/src/pages/TemplateSettingsPage/TemplateSettingsForm.tsx
    
    Co-authored-by: Cian Johnston <cian@coder.com>
    
    * ran prettier
    
    * added ttl of 0 test
    
    * typo
    
    * PR feedback
    
    Co-authored-by: Cian Johnston <cian@coder.com>

[33mcommit 4e3b2127070e85ef1bff3b6ecda9831e7f27fe90[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Wed Aug 24 13:54:45 2022 -0400

    make agent 'connecting' visually different from 'connected' (#3675)

[33mcommit 4f8270d95b46a086cb05029ed271b4564a5d8b87[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Wed Aug 24 12:04:33 2022 -0500

    fix: Exclude time column when selecting build log (#3673)
    
    Closes #2962.

[33mcommit 1400d7cd84708bcc6e255f477b15253fa9fe880e[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Wed Aug 24 12:49:03 2022 -0400

    fix: correctly link agent name in app urls (#3672)

[33mcommit ca3c0490e0a0a4e279eca0015d80c392d9509a33[m
Author: Eric Paulsen <ericpaulsen@coder.com>
Date:   Wed Aug 24 11:23:02 2022 -0500

    chore: k8s example persistence & coder images (#3619)
    
    * add: persistence & coder images
    
    * add: code-server
    
    * chore: README updates
    
    * chore: README example

[33mcommit 123fe0131eacef645c64c60226a64c097abc5906[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Wed Aug 24 19:12:40 2022 +0300

    fix: Avoid double escaping of ProxyCommand on Windows (#3664)
    
    Fixes #2853

[33mcommit 09142255e6be8b5d4be2c6516c93f88bc6e30f0f[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Wed Aug 24 10:40:36 2022 -0500

    fix: Add consistent use of `coder templates init` (#3665)
    
    Closes #2303.

[33mcommit 706bceb7e775c6e783f5b54e6434b68a9416c686[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Wed Aug 24 10:35:46 2022 -0500

    fix: Remove reference to `coder rebuild` command (#3670)
    
    Closes #2464.

[33mcommit eba753ba8713594436f2adfca1d074aa511f9fb3[m
Author: Cian Johnston <cian@coder.com>
Date:   Wed Aug 24 15:45:14 2022 +0100

    fix: template: enforce bounds of template max_ttl (#3662)
    
    This PR makes the following changes:
    
    - enforces lower and upper limits on template `max_ttl_ms`
    - adds a migration to enforce 7-day cap on `max_ttl`
    - allows setting template `max_ttl` to 0
    - updates template edit CLI help to be clearer

[33mcommit 343d1184b23b692579788a09b652584c07f96de1[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Wed Aug 24 16:58:46 2022 +0300

    fix: Clean up `coder config-ssh` dry-run behavior (#3660)
    
    This commit also drops old deprecated code.
    
    Fixes #2982

[33mcommit 7a71180ae68fbba42861bee981638ae665fd4d3b[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Wed Aug 24 15:44:30 2022 +0300

    chore: Enable comments for database dump / models (#3661)

[33mcommit 253e6cbffabcea6c2c35c8c68eb5b2b9cf8776e6[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Tue Aug 23 18:44:32 2022 -0500

    web: fix template permission check (#3652)
    
    Resolves #3582

[33mcommit 184f0625e15451edd7e2faeffb400ed54430875d[m
Author: Spike Curtis <spike@coder.com>
Date:   Tue Aug 23 13:55:39 2022 -0700

    coder licenses add CLI command (#3632)
    
    * coder licenses add CLI command
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Fix up lint
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Fix t.parallel call
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Code review improvements
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Lint
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 6dacf708988022b6181f4cf0c0285f7436029fa9[m
Author: Cian Johnston <cian@coder.com>
Date:   Tue Aug 23 21:19:26 2022 +0100

    fix: disable AccountForm when user is not allowed edit users (#3649)
    
    * RED: add unit tests for AccountForm username field
    * GREEN: disable username field and button on account form when user edits are not allowed
    
    Co-authored-by: Joe Previte <jjprevite@gmail.com>

[33mcommit b9dd5668043d241e939a8555e9354707e8c5c691[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Tue Aug 23 15:22:42 2022 -0400

    fix scrollbar on ssh key view (#3647)

[33mcommit e44f7adb7ede4bc73430269f75fa1af9e9d120d7[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Tue Aug 23 21:19:57 2022 +0300

    feat: Set SSH env vars: `SSH_CLIENT`, `SSH_CONNECTION` and `SSH_TTY` (#3622)
    
    Fixes #2339

[33mcommit 9c0cd5287cb7bd87036f90bec6255d65c95af6a0[m
Author: Garrett Delfosse <garrett@coder.com>
Date:   Tue Aug 23 13:30:46 2022 -0400

    fix: clarify we download templates on template select (#3296)
    
    Co-authored-by: Joe Previte <jjprevite@gmail.com>
    Co-authored-by: Steven Masley <Emyrk@users.noreply.github.com>

[33mcommit 5025fe2fa0683444b45dffa16fac98be162cf075[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Tue Aug 23 19:07:31 2022 +0300

    fix: Protect circular buffer during close in reconnectingPTY (#3646)

[33mcommit 49de44c76d3fcc74a3ef7e3e23653917691e5d8b[m
Author: Presley Pizzo <1290996+presleyp@users.noreply.github.com>
Date:   Tue Aug 23 11:26:22 2022 -0400

    feat: Add LicenseBanner (#3568)
    
    * Extract reusable Pill component
    
    * Make icon optional
    
    * Get pills in place
    
    * Rough styling
    
    * Extract Expander component
    
    * Fix alignment
    
    * Put it in action - type error
    
    * Hide banner by default
    
    * Use generated type
    
    * Move PaletteIndex type
    
    * Tweak colors
    
    * Format, another color tweak
    
    * Add stories
    
    * Add tests
    
    * Update site/src/components/Pill/Pill.tsx
    
    Co-authored-by: Kira Pilot <kira@coder.com>
    
    * Update site/src/components/Pill/Pill.tsx
    
    Co-authored-by: Kira Pilot <kira@coder.com>
    
    * Comments
    
    * Remove empty story, improve empty test
    
    * Lint
    
    Co-authored-by: Kira Pilot <kira@coder.com>

[33mcommit f7ccfa2ab931708795763107a3c9743a8da87e92[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Tue Aug 23 14:29:01 2022 +0300

    feat: Set `CODER=true` in workspaces (#3637)
    
    Fixes #2340

[33mcommit 8343a4f19924a9011f58e5cce2c6d75b544ada8d[m
Author: Colin Adler <colin1adler@gmail.com>
Date:   Mon Aug 22 22:40:11 2022 -0500

    chore: cleanup go.mod (#3636)

[33mcommit a7b49788f591d0f3c1dd2859ee2107a889317426[m
Author: Jon Ayers <jon@coder.com>
Date:   Mon Aug 22 18:13:46 2022 -0500

    chore: deduplicate OAuth login code (#3575)

[33mcommit a07ca946c3d01455a7fefe231c2e55cc5200ea83[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Mon Aug 22 17:24:15 2022 -0500

    Increase default auto-stop to 12h (#3631)
    
    Resolves #3462.
    
    And, clarify language to resolve #3509.

[33mcommit 8ca3fa97124c4a869844061df2b6bf441eba69d5[m
Author: Ben Potter <ben@coder.com>
Date:   Mon Aug 22 17:19:30 2022 -0500

    fix: use hardcoded "coder" user for AWS and Azure (#3625)

[33mcommit b101a6f3f499db2cf13ec2734aa33f868ed38e07[m
Author: Spike Curtis <spike@coder.com>
Date:   Mon Aug 22 15:02:50 2022 -0700

    POST license API endpoint (#3570)
    
    * POST license API
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Support interface{} types in generated Typescript
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Disable linting on empty interface any
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Code review updates
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Enforce unique licenses
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Renames from code review
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * Code review renames and comments
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 85acfdf0dc25d2a15f132c85d8b2351ea1aa373e[m
Author: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>
Date:   Mon Aug 22 16:56:39 2022 -0400

    chore: bump msw from 0.44.2 to 0.45.0 in /site (#3629)
    
    Bumps [msw](https://github.com/mswjs/msw) from 0.44.2 to 0.45.0.
    - [Release notes](https://github.com/mswjs/msw/releases)
    - [Changelog](https://github.com/mswjs/msw/blob/main/CHANGELOG.md)
    - [Commits](https://github.com/mswjs/msw/compare/v0.44.2...v0.45.0)
    
    ---
    updated-dependencies:
    - dependency-name: msw
      dependency-type: direct:development
      update-type: version-update:semver-minor
    ...
    
    Signed-off-by: dependabot[bot] <support@github.com>
    
    Signed-off-by: dependabot[bot] <support@github.com>
    Co-authored-by: dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>

[33mcommit 2ee6acb2ad0bffc968d8528e665d2705edd48ec4[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Mon Aug 22 15:42:06 2022 -0500

    Upgrade frontend to React 18 (#3353)
    
    Co-authored-by: Kira Pilot <kira.pilot23@gmail.com>

[33mcommit 6fde537f9cfc272de8f8c5c8c658dde1c24ba1ec[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Mon Aug 22 15:35:17 2022 -0500

    web: use seconds in max TTL input (#3576)
    
    Milliseconds are more difficult to deal with due to
    all of the zeros.
    
    Also, describe this feature as "auto-stop" to be
    consistent with our Workspace page UI and CLI. "ttl"
    is our backend lingo which should eventually be updated.

[33mcommit 5e36be8cbb11c71e840a40193ae93c589cdd16e2[m[33m ([m[1;33mtag: v0.8.6[m[33m)[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Mon Aug 22 10:56:10 2022 -0500

    docs: remove architecture diagram (#3615)
    
    The diagram was more confusion than helpful.

[33mcommit 58d29264aa11521e2afdf0ed8fcd7ba6093d6fe0[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Mon Aug 22 09:42:11 2022 -0500

    feat: Add template icon to the workspaces page (#3612)
    
    This removes the last built by column from the page. It seemed
    cluttered to have both on the page, and is simple enough to
    click on the workspace to see additional info.

[33mcommit 369a9fb535b2e9779cec163235d725c8ae7ae567[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Mon Aug 22 19:43:13 2022 +1000

    fix: add writeable home dir to docker image (#3603)

[33mcommit 68e17921f048d9d729ab4d4eb3d538f4e6b459fa[m
Author: Eric Paulsen <ericpaulsen@coder.com>
Date:   Sun Aug 21 18:50:36 2022 -0500

    fix: tooltip 404 (#3618)

[33mcommit b0fe9bcdd1cd162d7ad1d5c3bb09553d8afe9007[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Sun Aug 21 17:32:53 2022 -0500

    chore: Upgrade to Go 1.19 (#3617)
    
    This is required as part of #3505.

[33mcommit d37fb054c8afc4183d704baca9dbcbe99e1fe3d2[m
Author: Ammar Bandukwala <ammar@ammar.io>
Date:   Sat Aug 20 20:59:40 2022 -0500

    docs: outdent remote desktop docs (#3614)
    
    Resolves #3590

[33mcommit 54b8e794ce0c58a4582b733377b265791a073afb[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Aug 19 17:42:05 2022 -0300

    feat: Add emoji picker for template icons (#3601)

[33mcommit a4c90c591dd0642cd378b57bcdaaef9287bd99de[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Aug 19 15:37:16 2022 -0300

    feat: Add icon to the template page (#3604)

[33mcommit 690e6c6585c12295ced39594f8e86e8ba6d6b8bb[m
Author: Spike Curtis <spike@coder.com>
Date:   Fri Aug 19 10:49:08 2022 -0700

    Check AGPL code doesn't import enterprise (#3602)
    
    * Check AGPL code doesn't import enterprise
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    * use error/log instead of echo/exit
    
    Signed-off-by: Spike Curtis <spike@coder.com>
    
    Signed-off-by: Spike Curtis <spike@coder.com>

[33mcommit 91bfcca2870567f485e50ce2d5512da32c750de5[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Fri Aug 19 09:58:31 2022 -0700

    fix(ui): decrease WorkspaceActions popover padding (#3555)
    
    There was too much padding on the WorkspaceActions dropdown. This fixes
    that.

[33mcommit c14a4b92ed81a2a0dcfa607c10883e73748e50f2[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Aug 19 13:09:07 2022 -0300

    feat: Display and edit template icons in the UI (#3598)

[33mcommit e938e8577f7d43f2827ef5b1c04eb3a5cff60a8e[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Fri Aug 19 08:41:17 2022 -0700

    fix: add missing && \ in Dockerfile (#3594)
    
    * fix: add missing && \ in Dockerfile
    
    * fixup: add goboring after PATH goboring

[33mcommit 985eea6099c7ce16e8acad96d424e327f108c9b8[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Fri Aug 19 10:11:54 2022 -0500

    fix: Update icon when metadata is changed (#3587)
    
    This was causing names to become empty! Fixes #3586.

[33mcommit c417115eb19e8ede61e639a403e533dfe0ede9b8[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Fri Aug 19 08:10:56 2022 -0700

    feat: add cmake, nfpm to dogfood dockerfile (#3558)
    
    * feat: add cmake, nfpm to dogfood dockerfile
    
    * fixup: formatting
    
    * Update dogfood/Dockerfile
    
    Co-authored-by: Cian Johnston <cian@coder.com>
    
    Co-authored-by: Cian Johnston <cian@coder.com>

[33mcommit 544bf01fbbea0b8c60a3ea2ea71d169b13f8a92e[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Fri Aug 19 17:18:11 2022 +0300

    chore: Update `coder/coder` provider in example templates (#3581)
    
    Additionally, a convenience script was added to
    `examples/update_template_versions.sh` to keep the templates up-to-date.
    
    Fixes #2966

[33mcommit 80f042f01b104a52fe19b17bc8efeb5f08bd7d07[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Fri Aug 19 10:17:35 2022 -0300

    feat: Add icon to templates (#3561)

[33mcommit 57f3410009201df2038f06a65c796e9690c2e617[m
Author: Cian Johnston <cian@coder.com>
Date:   Fri Aug 19 11:08:56 2022 +0100

    cli: remove confirm prompt when starting a workspace (#3580)

[33mcommit 3fdae47b87e9ffbb9a02cdcdae28ee80888c534d[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Fri Aug 19 11:56:28 2022 +0300

    fix: Shadow err in TestProvision_Cancel to fix test race (#3579)
    
    Fixes #3574

[33mcommit 4ba3573632ef6568ed1f6b8f48b289d31a13af0b[m
Author: Eric Paulsen <ericpaulsen@coder.com>
Date:   Thu Aug 18 18:47:12 2022 -0500

    fix: quickstart 404 (#3564)

[33mcommit f6b0835982a9ffc3307fa9890b490ca54df8fe88[m
Author: Jon Ayers <jon@coder.com>
Date:   Thu Aug 18 17:56:17 2022 -0500

    fix: avoid processing updates to usernames (#3571)
    
    - With the support of OIDC we began processing updates to a user's
      email and username to stay in sync with the upstream provider. This
      can cause issues in templates that use the user's username as a stable
      identifier, potentially causing the deletion of user's home volumes.
    - Fix some faulty error wrapping.

[33mcommit 04c5f924d702fcea80fbee7d6816bd93f50a84c3[m
Author: Cian Johnston <cian@coder.com>
Date:   Thu Aug 18 23:32:23 2022 +0100

    fix: ui: workspace bumpers now honour template max_ttl (#3532)
    
    - chore: WorkspacePage: invert workspace schedule bumper logic for readibility
    - fix: make workspace bumpers honour template max_ttl
    - chore: refactor workspace schedule bumper logic to util/schedule.ts and unit test separately

[33mcommit 7599ad4bf61c00c99d93bd171144fabf53c1126b[m
Author: Bruno Quaresma <bruno@coder.com>
Date:   Thu Aug 18 16:58:01 2022 -0300

    feat: Add template settings page (#3557)

[33mcommit aabb72783c816efda7c76e94974150184012ef5e[m
Author: Joe Previte <jjprevite@gmail.com>
Date:   Thu Aug 18 10:11:58 2022 -0700

    docs: update CONTRIBUTING requirements (#3541)
    
    * docs: update CONTRIBUTING requirements
    
    * Update docs/CONTRIBUTING.md
    
    * refactor: remove dev from Makefile
    
    * fixup: add linux section

[33mcommit 55890df6f12b22ea1249d6f3dbafa86d011556d0[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Fri Aug 19 02:41:23 2022 +1000

    feat: add helm README, install guide, linters (#3268)

[33mcommit 3610402cd8d11957d38ce2be2f6f9e8f539e643a[m
Author: Dean Sheather <dean@deansheather.com>
Date:   Fri Aug 19 02:41:00 2022 +1000

    Use new table formatter everywhere (#3544)

[33mcommit c43297937be651a4e7babd272f4f943398866537[m
Author: Kyle Carberry <kyle@coder.com>
Date:   Thu Aug 18 10:57:46 2022 -0500

    feat: Add Kubernetes and resource metadata telemetry (#3548)
    
    Fixes #3524.

[33mcommit f1423450bda74ba1e5258bc11a09c3bf61e2ee89[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Thu Aug 18 17:03:55 2022 +0300

    fix: Allow terraform provisions to be gracefully cancelled (#3526)
    
    * fix: Allow terraform provisions to be gracefully cancelled
    
    This change allows terraform commands to be gracefully cancelled on
    Unix-like platforms by signaling interrupt on provision cancellation.
    
    One implementation detail to note is that we do not necessarily kill a
    running terraform command immediately even if the stream is closed. The
    reason for this is to allow for graceful cancellation even in such an
    event. Currently the timeout is set to 5 minutes by default.
    
    Related: #2683
    
    The above issue may be partially or fully fixed by this change.
    
    * fix: Remove incorrect minimumTerraformVersion variable
    
    * Allow init to return provision complete response

[33mcommit 6a0f8ae9ccdae90c54222b84224418aabeae273d[m
Author: Mathias Fredriksson <mafredri@gmail.com>
Date:   Thu Aug 18 16:25:32 2022 +0300

    fix: Add `SIGHUP` and `SIGTERM` handling to `coder server` (#3543)
    
    * fix: Add `SIGHUP` and `SIGTERM` handling to `coder server`
    
    To prevent additional signals from aborting program execution, signal
    handling was moved to the beginning of the main function, this ensures
    that signals stays registered for the entire shutdown procedure.
    
    Fixes #1529

[33mcommit 380022fe63d280e9bd22d8475db0da26444e2743[m
Author: Jon Ayers <jon@coder.com>
Date:   Wed Aug 17 23:06:03 2022 -0500

    fix: update oauth token on each login (#3542)

[33mcommit c3eea98db0d85bc0a2e61c6859d62d6e6a3592a8[m
Author: Jon Ayers <jon@coder.com>
Date:   Wed Aug 17 18:00:53 2022 -0500

    fix: use unique ID for linked accounts (#3441)
    
    - move OAuth-related fields off of api_keys into a new user_links table
    - restrict users to single form of login
    - process updates to user email/usernames for OIDC
    - added a login_type column to users

[33mcommit 53d1fb36db69d17edd06325bab3d9f2cdaf51293[m
Author: Cian Johnston <cian@coder.com>
Date:   Wed Aug 17 21:03:44 2022 +0100

    update-alternatives to ensure gofmt is goboring gofmt (#3540)
