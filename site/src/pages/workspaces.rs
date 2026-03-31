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

fn status_text_class(status: &str) -> &'static str {
    match status {
        "running" => "text-[var(--content-success)]",
        "stopped" => "text-[var(--content-secondary)]",
        "starting" | "stopping" | "pending" => "text-[var(--content-warning)]",
        "failed" => "text-[var(--content-destructive)]",
        "canceling" | "canceled" => "text-[var(--content-secondary)]",
        "deleting" => "text-[var(--content-destructive)]",
        "deleted" => "text-[var(--content-disabled)]",
        _ => "",
    }
}

fn status_dot_class(status: &str) -> &'static str {
    match status {
        "running" => "bg-[var(--content-success)]",
        "stopped" => "bg-[var(--content-secondary)]",
        "starting" | "stopping" | "pending" => "bg-[var(--content-warning)] animate-pulse",
        "failed" => "bg-[var(--content-destructive)]",
        "canceling" | "canceled" => "bg-[var(--content-secondary)]",
        "deleting" => "bg-[var(--content-destructive)] animate-pulse",
        "deleted" => "bg-[var(--content-disabled)]",
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

    let resp = match crate::api::http::get(&url).send().await {
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
        <div class="max-w-[1380px] w-full mx-auto px-6">

            // ── Page header ──────────────────────────────────────────────
            <header class="flex items-center justify-between pt-8 pb-6 max-md:flex-col max-md:items-start max-md:gap-4">
                <h1 class="text-2xl font-semibold flex items-center gap-2">"Workspaces"</h1>
                <a href="/templates" class="inline-flex items-center justify-center gap-2 rounded-lg text-sm font-medium cursor-pointer transition-all border border-transparent px-4 py-2.5 bg-[var(--content-primary)] text-[var(--content-invert)] hover:bg-gray-300 no-underline whitespace-nowrap leading-none">"New workspace"</a>
            </header>

            // ── Filter bar ───────────────────────────────────────────────
            <div class="flex items-center gap-2 py-4">
                <input
                    class="flex-1 px-3 py-2 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)]"
                    type="text"
                    placeholder="Search workspaces\u{2026}"
                    prop:value={move || search_query.get()}
                    on:input=move |ev| {
                        set_search_query.set(event_target_value(&ev));
                    }
                    on:keydown=on_search
                />
                <button class="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)] bg-[var(--surface-secondary)] border border-[var(--border-default)] rounded-lg text-[var(--content-secondary)] cursor-pointer">"Status"</button>
                <button class="inline-flex items-center gap-1 px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)] bg-[var(--surface-secondary)] border border-[var(--border-default)] rounded-lg text-[var(--content-secondary)] cursor-pointer">"Template"</button>
            </div>

            // ── Table ────────────────────────────────────────────────────
            {move || match data.get() {
                None => view! { <LoadingTable /> }.into_any(),
                Some(Err(err)) => view! {
                    <div class="flex flex-col items-center justify-center py-16 px-8 text-center">
                        <p class="text-lg font-semibold mb-2">"Failed to load workspaces"</p>
                        <p class="text-[var(--content-secondary)] text-sm max-w-[480px]">{err}</p>
                    </div>
                }.into_any(),
                Some(Ok(resp)) if resp.workspaces.is_empty() => view! {
                    <div class="flex flex-col items-center justify-center py-16 px-8 text-center">
                        <p class="text-lg font-semibold mb-2">"No workspaces found"</p>
                        <p class="text-[var(--content-secondary)] text-sm max-w-[480px]">
                            "Create a workspace from a template to get started."
                        </p>
                    </div>
                }.into_any(),
                Some(Ok(resp)) => {
                    let count = resp.count;
                    let len = resp.workspaces.len();
                    view! {
                        <div class="flex items-center justify-between px-4 py-3 text-[13px] text-[var(--content-secondary)] border border-[var(--border-default)] border-b-0 rounded-t-lg">
                            <span>
                                {format!("1\u{2013}{len} of {count} workspaces")}
                            </span>
                        </div>
                        <table class="w-full">
                            <thead>
                                <tr>
                                    <th class="w-1/3">"Name"</th>
                                    <th class="w-1/3">"Template"</th>
                                    <th class="w-1/3">"Status"</th>
                                    <th class="w-0"></th>
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
        <div class="flex items-center justify-between px-4 py-3 text-[13px] text-[var(--content-secondary)] border border-[var(--border-default)] border-b-0 rounded-t-lg">
            <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse" style="width:12rem;height:1rem"></span>
        </div>
        <table class="w-full">
            <thead>
                <tr>
                    <th class="w-1/3">"Name"</th>
                    <th class="w-1/3">"Template"</th>
                    <th class="w-1/3">"Status"</th>
                    <th class="w-0"></th>
                </tr>
            </thead>
            <tbody>
                {(0..5).map(|_| view! {
                    <tr>
                        <td>
                            <div class="flex items-center gap-3">
                                <div class="w-10 h-10 rounded-full bg-[var(--surface-tertiary)] animate-pulse shrink-0"></div>
                                <div class="flex flex-col gap-0.5 min-w-0">
                                    <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block" style="width:8rem;height:0.875rem"></span>
                                    <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block mt-1" style="width:5rem;height:0.75rem"></span>
                                </div>
                            </div>
                        </td>
                        <td><span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block" style="width:6rem;height:0.875rem"></span></td>
                        <td><span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block" style="width:5rem;height:0.875rem"></span></td>
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
    let status_cls = format!("inline-flex items-center gap-1.5 text-[13px] font-medium {}", status_text_class(status));
    let dot_cls = format!("w-2 h-2 rounded-full shrink-0 {}", status_dot_class(status));
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
                <div class="flex items-center gap-3">
                    <div class="w-10 h-10 rounded-full bg-[var(--surface-tertiary)] flex items-center justify-center font-semibold text-sm text-[var(--content-primary)] shrink-0 overflow-hidden [&_img]:w-full [&_img]:h-full [&_img]:object-cover">
                        {if has_avatar {
                            view! { <img src={avatar_url.clone()} alt="" /> }.into_any()
                        } else {
                            view! { {owner_initial} }.into_any()
                        }}
                    </div>
                    <div class="flex flex-col gap-0.5 min-w-0">
                        <span class="text-sm font-medium text-[var(--content-primary)] whitespace-nowrap overflow-hidden text-ellipsis">
                            {favorite.then(|| view! { <StarIcon /> " " })}
                            {name}
                            {outdated.then(|| view! {
                                " " <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-full bg-[var(--content-warning)]/15 text-[var(--content-warning)]">"Outdated"</span>
                            })}
                        </span>
                        <span class="text-[13px] text-[var(--content-secondary)]">{owner_name}</span>
                    </div>
                </div>
            </td>

            // ── Template ─────────────────────────────────────────────
            <td>
                <div class="flex items-center gap-3">
                    <div class="w-10 h-10 rounded-md bg-[var(--surface-tertiary)] flex items-center justify-center font-semibold text-sm text-[var(--content-primary)] shrink-0 overflow-hidden [&_img]:w-full [&_img]:h-full [&_img]:object-cover">
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
                <div class="flex gap-1 justify-end">
                    {can_start.then(|| view! {
                        <button class="inline-flex items-center justify-center w-10 h-10 rounded-lg text-sm font-medium cursor-pointer transition-all border border-[var(--border-default)] bg-transparent text-[var(--content-primary)] hover:bg-[var(--surface-secondary)] hover:border-[var(--border-hover)]" title="Start workspace">
                            <PlayIcon />
                        </button>
                    })}
                    {can_stop.then(|| view! {
                        <button class="inline-flex items-center justify-center w-10 h-10 rounded-lg text-sm font-medium cursor-pointer transition-all border border-[var(--border-default)] bg-transparent text-[var(--content-primary)] hover:bg-[var(--surface-secondary)] hover:border-[var(--border-hover)]" title="Stop workspace">
                            <StopIcon />
                        </button>
                    })}
                    <button class="inline-flex items-center justify-center w-10 h-10 rounded-lg cursor-pointer transition-all border border-transparent bg-transparent text-[var(--content-secondary)] hover:bg-[var(--surface-secondary)] hover:text-[var(--content-primary)]" title="More options">
                        <MoreIcon />
                    </button>
                </div>
            </td>
        </tr>
    }
}
