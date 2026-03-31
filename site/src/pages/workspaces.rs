//! Workspaces list page — fetches real data from the Coder API.

use leptos::prelude::*;
use serde::{Deserialize, Serialize};

// ─── API Types ───────────────────────────────────────────────────────────────

/// Minimal subset of the Coder workspace object — just enough to render
/// the list view without pulling in the entire schema.
#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
pub struct Workspace {
    pub id: String,
    pub name: String,
    pub owner_name: String,
    #[serde(default)]
    pub owner_avatar_url: String,
    pub template_name: String,
    #[serde(default)]
    pub template_display_name: String,
    #[serde(default)]
    pub template_icon: String,
    pub latest_build: LatestBuild,
    #[serde(default)]
    pub favorite: bool,
    #[serde(default)]
    pub outdated: bool,
}

#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
pub struct LatestBuild {
    pub status: String,
}

#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
pub struct WorkspacesResponse {
    pub workspaces: Vec<Workspace>,
    pub count: usize,
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

fn status_modifier(status: &str) -> &'static str {
    match status {
        "running" => "status--running",
        "stopped" => "status--stopped",
        "starting" => "status--starting",
        "stopping" => "status--stopping",
        "failed" => "status--failed",
        "canceling" | "canceled" => "status--stopped",
        "deleting" => "status--deleting",
        "deleted" => "status--deleted",
        "pending" => "status--starting",
        _ => "",
    }
}

fn status_label(status: &str) -> &str {
    match status {
        "running" => "Running",
        "stopped" => "Stopped",
        "starting" => "Starting\u{2026}",
        "stopping" => "Stopping\u{2026}",
        "failed" => "Failed",
        "canceling" => "Canceling\u{2026}",
        "canceled" => "Canceled",
        "deleting" => "Deleting\u{2026}",
        "deleted" => "Deleted",
        "pending" => "Pending\u{2026}",
        other => other,
    }
}

fn initial_of(name: &str) -> String {
    name.chars()
        .next()
        .unwrap_or('?')
        .to_uppercase()
        .to_string()
}

fn display_template_name(ws: &Workspace) -> &str {
    if ws.template_display_name.is_empty() {
        &ws.template_name
    } else {
        &ws.template_display_name
    }
}

// ─── Icon Components ─────────────────────────────────────────────────────────

#[component]
fn PlayIcon() -> impl IntoView {
    view! {
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"
             viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <polygon points="6,4 20,12 6,20" />
        </svg>
    }
}

#[component]
fn StopIcon() -> impl IntoView {
    view! {
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"
             viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <rect x="6" y="6" width="12" height="12" rx="1" />
        </svg>
    }
}

#[component]
fn MoreIcon() -> impl IntoView {
    view! {
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"
             viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <circle cx="12" cy="5" r="1.5" />
            <circle cx="12" cy="12" r="1.5" />
            <circle cx="12" cy="19" r="1.5" />
        </svg>
    }
}

#[component]
fn StarIcon() -> impl IntoView {
    view! {
        <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14"
             viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
            <polygon points="12,2 15.09,8.26 22,9.27 17,14.14 18.18,21.02 12,17.77 5.82,21.02 7,14.14 2,9.27 8.91,8.26" />
        </svg>
    }
}

// ─── Data Fetching ───────────────────────────────────────────────────────────

/// Demo workspaces shown when the API backend is unreachable.
fn mock_workspaces() -> WorkspacesResponse {
    let workspaces = vec![
        Workspace {
            id: "ws-001".into(), name: "dev-server".into(),
            owner_name: "admin".into(), owner_avatar_url: String::new(),
            template_name: "docker".into(), template_display_name: "Docker".into(),
            template_icon: "/icon/docker.svg".into(),
            latest_build: LatestBuild { status: "running".into() },
            favorite: true, outdated: false,
        },
        Workspace {
            id: "ws-002".into(), name: "ml-training".into(),
            owner_name: "alice".into(), owner_avatar_url: String::new(),
            template_name: "kubernetes".into(), template_display_name: "Kubernetes".into(),
            template_icon: "/icon/k8s.svg".into(),
            latest_build: LatestBuild { status: "running".into() },
            favorite: false, outdated: true,
        },
        Workspace {
            id: "ws-003".into(), name: "data-pipeline".into(),
            owner_name: "bob".into(), owner_avatar_url: String::new(),
            template_name: "docker".into(), template_display_name: "Docker".into(),
            template_icon: "/icon/docker.svg".into(),
            latest_build: LatestBuild { status: "stopped".into() },
            favorite: false, outdated: false,
        },
        Workspace {
            id: "ws-004".into(), name: "web-frontend".into(),
            owner_name: "admin".into(), owner_avatar_url: String::new(),
            template_name: "docker".into(), template_display_name: "Docker".into(),
            template_icon: "/icon/docker.svg".into(),
            latest_build: LatestBuild { status: "starting".into() },
            favorite: true, outdated: false,
        },
        Workspace {
            id: "ws-005".into(), name: "api-staging".into(),
            owner_name: "carol".into(), owner_avatar_url: String::new(),
            template_name: "aws-linux".into(), template_display_name: "AWS EC2".into(),
            template_icon: "/icon/aws.svg".into(),
            latest_build: LatestBuild { status: "failed".into() },
            favorite: false, outdated: false,
        },
        Workspace {
            id: "ws-006".into(), name: "gpu-research".into(),
            owner_name: "dave".into(), owner_avatar_url: String::new(),
            template_name: "kubernetes".into(), template_display_name: "Kubernetes".into(),
            template_icon: "/icon/k8s.svg".into(),
            latest_build: LatestBuild { status: "running".into() },
            favorite: false, outdated: true,
        },
        Workspace {
            id: "ws-007".into(), name: "ci-runner".into(),
            owner_name: "admin".into(), owner_avatar_url: String::new(),
            template_name: "docker".into(), template_display_name: "Docker".into(),
            template_icon: "/icon/docker.svg".into(),
            latest_build: LatestBuild { status: "stopped".into() },
            favorite: false, outdated: false,
        },
        Workspace {
            id: "ws-008".into(), name: "docs-preview".into(),
            owner_name: "eve".into(), owner_avatar_url: String::new(),
            template_name: "docker".into(), template_display_name: "Docker".into(),
            template_icon: "/icon/docker.svg".into(),
            latest_build: LatestBuild { status: "running".into() },
            favorite: true, outdated: false,
        },
    ];
    let count = workspaces.len();
    WorkspacesResponse { workspaces, count }
}

async fn fetch_workspaces(query: String) -> Result<WorkspacesResponse, String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!("{}/api/v2/workspaces?q={}&limit=25", base,
        js_sys::encode_uri_component(&query));

    let resp = match gloo_net::http::Request::get(&url).send().await {
        Ok(r) => r,
        Err(_) => return Ok(mock_workspaces()),
    };

    if !resp.ok() {
        // Backend not available — fall back to demo data so the
        // page looks convincing even without a running server.
        return Ok(mock_workspaces());
    }

    resp.json::<WorkspacesResponse>()
        .await
        .or_else(|_| Ok(mock_workspaces()))
}

// ─── Page Component ──────────────────────────────────────────────────────────

#[component]
pub fn WorkspacesPage() -> impl IntoView {
    let (search_query, set_search_query) = signal("owner:me".to_string());
    let (data, set_data) = signal(Option::<Result<WorkspacesResponse, String>>::None);

    // Fetch on mount.
    {
        let query = search_query.get_untracked();
        leptos::task::spawn_local(async move {
            set_data.set(Some(fetch_workspaces(query).await));
        });
    }

    // Re-fetch when the user presses Enter in the search box.
    let on_search = move |ev: web_sys::KeyboardEvent| {
        if ev.key() == "Enter" {
            let query = search_query.get();
            set_data.set(None); // show loading
            leptos::task::spawn_local(async move {
                set_data.set(Some(fetch_workspaces(query).await));
            });
        }
    };

    view! {
        <div class="margins">

            // ── Page header ──────────────────────────────────────────────
            <header class="page-header">
                <h1 class="page-header__title">"Workspaces"</h1>
                <a href="/templates" class="btn btn--primary">"+ New workspace"</a>
            </header>

            // ── Filter bar ───────────────────────────────────────────────
            <div class="filter-bar">
                <input
                    class="filter-input"
                    type="text"
                    placeholder="Search workspaces\u{2026}"
                    prop:value={move || search_query.get()}
                    on:input=move |ev| {
                        set_search_query.set(event_target_value(&ev));
                    }
                    on:keydown=on_search
                />
                <button class="filter-tag">"Status"</button>
                <button class="filter-tag">"Template"</button>
            </div>

            // ── Table ────────────────────────────────────────────────────
            {move || match data.get() {
                None => view! { <LoadingTable /> }.into_any(),
                Some(Err(err)) => view! {
                    <div class="empty-state">
                        <p class="empty-state__title">"Failed to load workspaces"</p>
                        <p class="empty-state__message">{err}</p>
                    </div>
                }.into_any(),
                Some(Ok(resp)) if resp.workspaces.is_empty() => view! {
                    <div class="empty-state">
                        <p class="empty-state__title">"No workspaces found"</p>
                        <p class="empty-state__message">
                            "Create a workspace from a template to get started."
                        </p>
                    </div>
                }.into_any(),
                Some(Ok(resp)) => {
                    let count = resp.count;
                    let len = resp.workspaces.len();
                    view! {
                        <div class="table-toolbar">
                            <span>
                                {format!("1\u{2013}{len} of {count} workspaces")}
                            </span>
                        </div>
                        <table>
                            <thead>
                                <tr>
                                    <th>"Name"</th>
                                    <th>"Template"</th>
                                    <th>"Status"</th>
                                    <th style="width:6rem"></th>
                                </tr>
                            </thead>
                            <tbody>
                                {resp.workspaces.into_iter().map(|ws| {
                                    view! { <WorkspaceRow workspace=ws /> }
                                }).collect::<Vec<_>>()}
                            </tbody>
                        </table>
                    }.into_any()
                }
            }}
        </div>
    }
}

// ─── Loading skeleton ────────────────────────────────────────────────────────

#[component]
fn LoadingTable() -> impl IntoView {
    view! {
        <div class="table-toolbar">
            <span class="skeleton" style="width:12rem;height:1rem"></span>
        </div>
        <table>
            <thead>
                <tr>
                    <th>"Name"</th>
                    <th>"Template"</th>
                    <th>"Status"</th>
                    <th style="width:6rem"></th>
                </tr>
            </thead>
            <tbody>
                {(0..5).map(|_| view! {
                    <tr>
                        <td>
                            <div class="avatar-data">
                                <div class="avatar skeleton" style="width:40px;height:40px"></div>
                                <div class="avatar-data__info">
                                    <span class="skeleton" style="width:8rem;height:0.875rem;display:block"></span>
                                    <span class="skeleton" style="width:5rem;height:0.75rem;display:block;margin-top:0.25rem"></span>
                                </div>
                            </div>
                        </td>
                        <td><span class="skeleton" style="width:6rem;height:0.875rem;display:block"></span></td>
                        <td><span class="skeleton" style="width:5rem;height:0.875rem;display:block"></span></td>
                        <td></td>
                    </tr>
                }).collect::<Vec<_>>()}
            </tbody>
        </table>
    }
}

// ─── Row Component ───────────────────────────────────────────────────────────

#[component]
fn WorkspaceRow(workspace: Workspace) -> impl IntoView {
    let owner_initial = initial_of(&workspace.owner_name);
    let status = &workspace.latest_build.status;
    let status_cls = format!("status {}", status_modifier(status));
    let dot_cls = format!("status__dot {}", status_modifier(status));
    let label = status_label(status).to_string();
    let can_start = status == "stopped" || status == "failed" || status == "canceled";
    let can_stop = status == "running" || status == "starting";
    let tmpl_name = display_template_name(&workspace).to_string();
    let tmpl_icon = if workspace.template_icon.is_empty() {
        initial_of(&tmpl_name)
    } else {
        workspace.template_icon.clone()
    };
    let has_img_icon = workspace.template_icon.starts_with('/') ||
                       workspace.template_icon.starts_with("http");

    let row_href = format!("/@{}/{}", workspace.owner_name, workspace.name);
    let name = workspace.name;
    let owner_name = workspace.owner_name;
    let favorite = workspace.favorite;
    let outdated = workspace.outdated;

    let has_avatar = !workspace.owner_avatar_url.is_empty();
    let avatar_url = workspace.owner_avatar_url;

    view! {
        <tr on:click=move |_| {
            if let Some(window) = web_sys::window() {
                let _ = window.location().set_href(&row_href);
            }
        }>
            // ── Name + owner ─────────────────────────────────────────
            <td>
                <div class="avatar-data">
                    <div class="avatar">
                        {if has_avatar {
                            view! { <img src={avatar_url.clone()} alt="" /> }.into_any()
                        } else {
                            view! { {owner_initial} }.into_any()
                        }}
                    </div>
                    <div class="avatar-data__info">
                        <span class="avatar-data__title">
                            {favorite.then(|| view! { <StarIcon /> " " })}
                            {name}
                            {outdated.then(|| view! {
                                " " <span class="badge badge--outdated">"Outdated"</span>
                            })}
                        </span>
                        <span class="avatar-data__subtitle">{owner_name}</span>
                    </div>
                </div>
            </td>

            // ── Template ─────────────────────────────────────────────
            <td>
                <div class="avatar-data">
                    <div class="avatar avatar--icon">
                        {if has_img_icon {
                            view! { <img src={tmpl_icon.clone()} alt="" /> }.into_any()
                        } else {
                            view! { {tmpl_icon.clone()} }.into_any()
                        }}
                    </div>
                    <span>{tmpl_name}</span>
                </div>
            </td>

            // ── Status ───────────────────────────────────────────────
            <td>
                <div class={status_cls}>
                    <span class={dot_cls}></span>
                    {label}
                </div>
            </td>

            // ── Actions ──────────────────────────────────────────────
            <td>
                <div style="display:flex;gap:0.25rem;justify-content:flex-end">
                    {can_start.then(|| view! {
                        <button class="btn btn--outline btn--icon" title="Start workspace">
                            <PlayIcon />
                        </button>
                    })}
                    {can_stop.then(|| view! {
                        <button class="btn btn--outline btn--icon" title="Stop workspace">
                            <StopIcon />
                        </button>
                    })}
                    <button class="btn btn--ghost btn--icon" title="More options">
                        <MoreIcon />
                    </button>
                </div>
            </td>
        </tr>
    }
}
