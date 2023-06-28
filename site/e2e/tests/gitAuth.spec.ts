import { test } from "@playwright/test"
import { gitAuth } from "../constants"
import { Endpoints } from "@octokit/types"
import { GitAuthDevice } from "api/typesGenerated"
import { Awaiter, createServer } from "../helpers"

test("git auth device", async ({ page }) => {
  const device: GitAuthDevice = {
    device_code: "1234",
    user_code: "1234-5678",
    expires_in: 900,
    interval: 1,
    verification_uri: "",
  }

  const srv = await createServer(gitAuth.devicePort)
  // The GitHub validate endpoint returns the currently authenticated user!
  srv.use(gitAuth.validatePath, (req, res) => {
    res.write(JSON.stringify(ghUser))
    res.end()
  })
  srv.use(gitAuth.codePath, (req, res) => {
    res.write(JSON.stringify(device))
    res.end()
  })
  srv.use(gitAuth.installationsPath, (req, res) => {
    res.write(JSON.stringify(ghInstall))
    res.end()
  })

  const token = {
    access_token: "",
    error: "authorization_pending",
    error_description: "",
  }

  const sentPending = new Awaiter()
  srv.use(gitAuth.tokenPath, (req, res) => {
    res.write(JSON.stringify(token))
    res.end()
    sentPending.done()
  })

  await page.goto(`/gitauth/${gitAuth.deviceProvider}`, {
    waitUntil: "networkidle",
  })
  await page.getByText(device.user_code).isVisible()
  await sentPending.wait()
  token.error = ""
  token.access_token = "hello-world"
  await page.getByText("1 organization authorized").isVisible()
})

test("git auth web", async ({ page }) => {
  const srv = await createServer(gitAuth.webPort)
  // The GitHub validate endpoint returns the currently authenticated user!
  srv.use(gitAuth.validatePath, (req, res) => {
    res.write(JSON.stringify(ghUser))
    res.end()
  })
  srv.use(gitAuth.installationsPath, (req, res) => {
    res.write(JSON.stringify(ghInstall))
    res.end()
  })
  srv.use(gitAuth.tokenPath, (req, res) => {
    res.write(JSON.stringify({ access_token: "hello-world" }))
    res.end()
  })

  await page.goto(`/gitauth/${gitAuth.webProvider}`, {
    waitUntil: "networkidle",
  })
  await page.getByText("1 organization authorized").isVisible()
})

const ghUser: Endpoints["GET /user"]["response"]["data"] = {
  login: "kylecarbs",
  id: 7122116,
  node_id: "MDQ6VXNlcjcxMjIxMTY=",
  avatar_url: "https://avatars.githubusercontent.com/u/7122116?v=4",
  gravatar_id: "",
  url: "https://api.github.com/users/kylecarbs",
  html_url: "https://github.com/kylecarbs",
  followers_url: "https://api.github.com/users/kylecarbs/followers",
  following_url:
    "https://api.github.com/users/kylecarbs/following{/other_user}",
  gists_url: "https://api.github.com/users/kylecarbs/gists{/gist_id}",
  starred_url: "https://api.github.com/users/kylecarbs/starred{/owner}{/repo}",
  subscriptions_url: "https://api.github.com/users/kylecarbs/subscriptions",
  organizations_url: "https://api.github.com/users/kylecarbs/orgs",
  repos_url: "https://api.github.com/users/kylecarbs/repos",
  events_url: "https://api.github.com/users/kylecarbs/events{/privacy}",
  received_events_url: "https://api.github.com/users/kylecarbs/received_events",
  type: "User",
  site_admin: false,
  name: "Kyle Carberry",
  company: "@coder",
  blog: "https://carberry.com",
  location: "Austin, TX",
  email: "kyle@carberry.com",
  hireable: null,
  bio: "hey there",
  twitter_username: "kylecarbs",
  public_repos: 52,
  public_gists: 9,
  followers: 208,
  following: 31,
  created_at: "2014-04-01T02:24:41Z",
  updated_at: "2023-06-26T13:03:09Z",
}

const ghInstall: Endpoints["GET /user/installations"]["response"]["data"] = {
  installations: [
    {
      id: 1,
      access_tokens_url: "",
      account: ghUser,
      app_id: 1,
      app_slug: "coder",
      created_at: "2014-04-01T02:24:41Z",
      events: [],
      html_url: "",
      permissions: {},
      repositories_url: "",
      repository_selection: "all",
      single_file_name: "",
      suspended_at: null,
      suspended_by: null,
      target_id: 1,
      target_type: "",
      updated_at: "2023-06-26T13:03:09Z",
    },
  ],
  total_count: 1,
}
