//! Workspace detail page — shows status, resources, and action buttons.
//!
//! Mounted inside `DashboardLayout` and reads the workspace owner and
//! name from query parameters (`?owner=X&name=Y`).

use leptos::prelude::*;
use leptos_router::hooks::use_query_map;
use serde::{Deserialize, Serialize};

// ─── API Types ───────────────────────────────────────────────────────────────

/// Full workspace object with enough detail for the single-workspace
/// view, including latest build resources and agents.
#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
struct Workspace {
    id: String,
    name: String,
    owner_name: String,
    #[serde(default)]
    template_name: String,
    #[serde(default)]
    template_display_name: String,
    #[serde(default)]
    template_icon: String,
    #[serde(default)]
    template_active_version_id: String,
    latest_build: LatestBuild,
    #[serde(default)]
    last_used_at: String,
    #[serde(default)]
    outdated: bool,
}

#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
struct LatestBuild {
    id: String,
    status: String,
    #[serde(default)]
    resources: Vec<Resource>,
}

#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
struct Resource {
    id: String,
    name: String,
    #[serde(rename = "type")]
    resource_type: String,
    #[serde(default)]
    agents: Option<Vec<Agent>>,
}

#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
struct Agent {
    id: String,
    name: String,
    status: String,
    #[serde(default)]
    operating_system: String,
    #[serde(default)]
    architecture: String,
    #[serde(default)]
    apps: Vec<App>,
}

#[derive(Clone, Debug, PartialEq, Deserialize, Serialize)]
struct App {
    #[serde(default)]
    display_name: String,
    #[serde(default)]
    icon: String,
    #[serde(default)]
    url: String,
}

#[derive(Serialize)]
struct CreateBuildRequest {
    transition: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    template_version_id: Option<String>,
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
        "starting" | "stopping" | "pending" => {
            "bg-[var(--content-warning)] animate-pulse"
        }
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

fn agent_status_dot(status: &str) -> &'static str {
    match status {
        "connected" => "bg-[var(--content-success)]",
        "connecting" => "bg-[var(--content-warning)] animate-pulse",
        "disconnected" | "timeout" => "bg-[var(--content-secondary)]",
        _ => "bg-[var(--content-secondary)]",
    }
}

fn agent_status_label(status: &str) -> &str {
    match status {
        "connected" => "Connected",
        "connecting" => "Connecting\u{2026}",
        "disconnected" => "Disconnected",
        "timeout" => "Timeout",
        other => other,
    }
}

fn display_template_name(ws: &Workspace) -> &str {
    if ws.template_display_name.is_empty() {
        &ws.template_name
    } else {
        &ws.template_display_name
    }
}

/// Formats an RFC 3339 timestamp into a readable
/// `YYYY-MM-DD HH:MM:SS` string, returning an em-dash when the
/// input is empty.
fn format_timestamp(ts: &str) -> String {
    if ts.is_empty() {
        return "\u{2014}".to_string();
    }
    // Show the first 19 characters (YYYY-MM-DDTHH:MM:SS) with a
    // space replacing the `T` separator.
    if ts.len() >= 19 {
        format!("{} {}", &ts[..10], &ts[11..19])
    } else {
        ts.to_string()
    }
}

// ─── Icon Components ─────────────────────────────────────────────────────────

#[component]
fn BackArrowIcon() -> impl IntoView {
    view! {
        <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20"
             viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
             aria-hidden="true">
            <path d="M19 12H5" />
            <path d="M12 19l-7-7 7-7" />
        </svg>
    }
}

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
fn TerminalIcon() -> impl IntoView {
    view! {
        <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"
             viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
             aria-hidden="true">
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
        </svg>
    }
}

// ─── Data Fetching ───────────────────────────────────────────────────────────

async fn fetch_workspace(
    owner: &str,
    name: &str,
) -> Result<Workspace, String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!(
        "{}/api/v2/users/{}/workspace/{}",
        base,
        js_sys::encode_uri_component(owner),
        js_sys::encode_uri_component(name),
    );

    let resp = crate::api::http::get(&url)
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if !resp.ok() {
        let text = resp.text().await.unwrap_or_default();
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&text)
        {
            if let Some(msg) =
                v.get("message").and_then(|m| m.as_str())
            {
                return Err(msg.to_string());
            }
        }
        return Err(format!(
            "Failed to load workspace ({})",
            resp.status()
        ));
    }

    resp.json::<Workspace>()
        .await
        .map_err(|e| format!("Failed to parse workspace: {e}"))
}

async fn post_workspace_build(
    workspace_id: &str,
    transition: &str,
    template_version_id: Option<String>,
) -> Result<(), String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!(
        "{}/api/v2/workspaces/{}/builds",
        base, workspace_id
    );

    let body = CreateBuildRequest {
        transition: transition.to_string(),
        template_version_id,
    };

    let resp = crate::api::http::post(&url)
        .body(serde_json::to_string(&body).unwrap())
        .map_err(|e| format!("Request build error: {e}"))?
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if resp.ok() {
        Ok(())
    } else {
        let text = resp.text().await.unwrap_or_default();
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&text)
        {
            if let Some(msg) =
                v.get("message").and_then(|m| m.as_str())
            {
                return Err(msg.to_string());
            }
        }
        Err(format!("Build request failed ({})", resp.status()))
    }
}

// ─── Page Component ──────────────────────────────────────────────────────────

#[component]
pub fn WorkspacePage() -> impl IntoView {
    let query_map = use_query_map();

    // Reactive closures that read the current query params. All
    // captured values (`Memo`, `WriteSignal`) are `Copy`, making
    // these closures `Copy` so they can be freely shared across
    // handlers and the reactive view closure.
    let owner =
        move || query_map.with(|m| m.get("owner").unwrap_or_default());
    let ws_name =
        move || query_map.with(|m| m.get("name").unwrap_or_default());

    // None = loading, Some(Ok(..)) = loaded, Some(Err(..)) = error.
    let (workspace, set_workspace) =
        signal(Option::<Result<Workspace, String>>::None);
    let (action_loading, set_action_loading) = signal(false);
    let (action_error, set_action_error) =
        signal(Option::<String>::None);

    // Fetch workspace on mount.
    {
        let o = owner();
        let n = ws_name();
        leptos::task::spawn_local(async move {
            if o.is_empty() || n.is_empty() {
                set_workspace.set(Some(Err(
                    "Missing owner or workspace name in query \
                     parameters."
                        .into(),
                )));
                return;
            }
            set_workspace
                .set(Some(fetch_workspace(&o, &n).await));
        });
    }

    // Re-fetch after a start / stop action completes.
    let refetch = move || {
        let o = owner();
        let n = ws_name();
        leptos::task::spawn_local(async move {
            set_workspace
                .set(Some(fetch_workspace(&o, &n).await));
        });
    };

    // ── Start workspace ──────────────────────────────────────────
    let on_start = move |_| {
        let ws = match workspace.get() {
            Some(Ok(ws)) => ws,
            _ => return,
        };
        let id = ws.id.clone();
        let ver = if ws.template_active_version_id.is_empty() {
            None
        } else {
            Some(ws.template_active_version_id.clone())
        };

        set_action_loading.set(true);
        set_action_error.set(None);

        leptos::task::spawn_local(async move {
            match post_workspace_build(&id, "start", ver).await {
                Ok(()) => {
                    set_action_loading.set(false);
                    refetch();
                }
                Err(e) => {
                    set_action_error.set(Some(e));
                    set_action_loading.set(false);
                }
            }
        });
    };

    // ── Stop workspace ───────────────────────────────────────────
    let on_stop = move |_| {
        let ws = match workspace.get() {
            Some(Ok(ws)) => ws,
            _ => return,
        };
        let id = ws.id.clone();

        set_action_loading.set(true);
        set_action_error.set(None);

        leptos::task::spawn_local(async move {
            match post_workspace_build(&id, "stop", None).await {
                Ok(()) => {
                    set_action_loading.set(false);
                    refetch();
                }
                Err(e) => {
                    set_action_error.set(Some(e));
                    set_action_loading.set(false);
                }
            }
        });
    };

    // ── View ─────────────────────────────────────────────────────
    view! {
        {move || match workspace.get() {
            // ── Loading ──────────────────────────────────────────
            None => view! { <WorkspacePageSkeleton /> }.into_any(),

            // ── Error ────────────────────────────────────────────
            Some(Err(err)) => view! {
                <div class="flex flex-col items-center justify-center py-24 px-8 text-center">
                    <p class="text-lg font-semibold mb-2">
                        "Failed to load workspace"
                    </p>
                    <p class="text-[var(--content-secondary)] text-sm max-w-[480px] mb-6">
                        {err}
                    </p>
                    <a href="/workspaces"
                       class="inline-flex items-center gap-2 text-sm text-[var(--content-secondary)] hover:text-[var(--content-primary)] no-underline">
                        <BackArrowIcon />
                        "Back to workspaces"
                    </a>
                </div>
            }.into_any(),

            // ── Loaded ───────────────────────────────────────────
            Some(Ok(ws)) => {
                // Extract everything we need before the view!
                // macro so there are no partial-move issues.
                let status = ws.latest_build.status.clone();
                let can_start = matches!(
                    status.as_str(),
                    "stopped" | "failed" | "canceled"
                );
                let can_stop = matches!(
                    status.as_str(),
                    "running" | "starting"
                );
                let is_transitioning = matches!(
                    status.as_str(),
                    "starting" | "stopping" | "canceling"
                        | "deleting" | "pending"
                );

                let st_text = status_text_class(&status);
                let st_dot = status_dot_class(&status);
                let st_label = status_label(&status).to_string();

                let tmpl = display_template_name(&ws).to_string();
                let has_tmpl_img = ws.template_icon.starts_with('/')
                    || ws.template_icon.starts_with("http");
                let tmpl_icon = ws.template_icon.clone();
                let tmpl_initial = tmpl
                    .chars()
                    .next()
                    .unwrap_or('?')
                    .to_uppercase()
                    .to_string();
                let last_used = format_timestamp(&ws.last_used_at);
                let build_id_short = if ws.latest_build.id.len() > 8 {
                    ws.latest_build.id[..8].to_string()
                } else {
                    ws.latest_build.id.clone()
                };
                let outdated = ws.outdated;
                let name_display = ws.name.clone();
                let owner_display = ws.owner_name.clone();

                // Flatten agents across all resources so we can
                // show them in a single list.
                let agents: Vec<(String, Agent)> = ws
                    .latest_build
                    .resources
                    .iter()
                    .flat_map(|r| {
                        r.agents
                            .as_deref()
                            .unwrap_or_default()
                            .iter()
                            .map(|a| (r.name.clone(), a.clone()))
                    })
                    .collect();

                view! {
                    // ── Topbar ────────────────────────────────────
                    <div class="border-b border-[var(--border-default)] px-6 py-4">
                        <div class="max-w-[1380px] mx-auto flex items-center justify-between gap-4">
                            // Left: back link + workspace identity
                            <div class="flex items-center gap-4 min-w-0">
                                <a href="/workspaces"
                                   class="text-[var(--content-secondary)] hover:text-[var(--content-primary)] transition-colors shrink-0 no-underline"
                                   title="Back to workspaces">
                                    <BackArrowIcon />
                                </a>
                                <div class="flex flex-col gap-0.5 min-w-0">
                                    <h1 class="text-xl font-semibold truncate">
                                        {name_display}
                                    </h1>
                                    <span class="text-sm text-[var(--content-secondary)]">
                                        {owner_display}
                                    </span>
                                </div>
                            </div>

                            // Right: status indicator + action buttons
                            <div class="flex items-center gap-3 shrink-0">
                                <div class={format!(
                                    "inline-flex items-center gap-1.5 text-sm font-medium {}",
                                    st_text
                                )}>
                                    <span class={format!(
                                        "w-2 h-2 rounded-full shrink-0 {}",
                                        st_dot
                                    )}></span>
                                    {st_label}
                                </div>

                                {can_start.then(|| view! {
                                    <button
                                        class="inline-flex items-center justify-center gap-2 h-9 px-4 rounded-lg text-sm font-medium cursor-pointer transition-all border border-[var(--border-default)] bg-transparent text-[var(--content-primary)] hover:bg-[var(--surface-secondary)] hover:border-[var(--border-hover)] disabled:opacity-50 disabled:cursor-not-allowed"
                                        title="Start workspace"
                                        disabled=move || action_loading.get()
                                        on:click=on_start
                                    >
                                        <Show
                                            when=move || action_loading.get()
                                            fallback=|| view! { <PlayIcon /> " Start" }
                                        >
                                            <span class="inline-block w-4 h-4 border-2 border-[var(--border-default)] border-t-[var(--content-primary)] rounded-full animate-spin"></span>
                                            " Starting\u{2026}"
                                        </Show>
                                    </button>
                                })}

                                {can_stop.then(|| view! {
                                    <button
                                        class="inline-flex items-center justify-center gap-2 h-9 px-4 rounded-lg text-sm font-medium cursor-pointer transition-all border border-[var(--border-default)] bg-transparent text-[var(--content-primary)] hover:bg-[var(--surface-secondary)] hover:border-[var(--border-hover)] disabled:opacity-50 disabled:cursor-not-allowed"
                                        title="Stop workspace"
                                        disabled=move || action_loading.get()
                                        on:click=on_stop
                                    >
                                        <Show
                                            when=move || action_loading.get()
                                            fallback=|| view! { <StopIcon /> " Stop" }
                                        >
                                            <span class="inline-block w-4 h-4 border-2 border-[var(--border-default)] border-t-[var(--content-primary)] rounded-full animate-spin"></span>
                                            " Stopping\u{2026}"
                                        </Show>
                                    </button>
                                })}

                                {is_transitioning.then(|| view! {
                                    <span class="inline-block w-5 h-5 border-2 border-[var(--border-default)] border-t-[var(--content-primary)] rounded-full animate-spin"></span>
                                })}
                            </div>
                        </div>
                    </div>

                    // ── Action error banner ──────────────────────
                    {move || action_error.get().map(|err| view! {
                        <div class="max-w-[1380px] mx-auto px-6 pt-4">
                            <div class="px-4 py-3 rounded-lg bg-red-950 border border-red-400 text-red-400 text-sm">
                                {err}
                            </div>
                        </div>
                    })}

                    // ── Content area ─────────────────────────────
                    <div class="max-w-[1380px] mx-auto px-6 py-6">
                        <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">

                            // ── Details card (left column) ───────
                            <div class="lg:col-span-1">
                                <div class="border border-[var(--border-default)] rounded-lg p-4">
                                    <h2 class="text-sm font-semibold text-[var(--content-secondary)] uppercase tracking-wider mb-4">
                                        "Details"
                                    </h2>

                                    // Template icon + name
                                    <div class="flex items-center gap-3 mb-4">
                                        <div class="w-10 h-10 rounded-md bg-[var(--surface-tertiary)] flex items-center justify-center text-sm font-semibold text-[var(--content-primary)] shrink-0 overflow-hidden [&_img]:w-full [&_img]:h-full [&_img]:object-cover">
                                            {if has_tmpl_img {
                                                view! {
                                                    <img src={tmpl_icon} alt="" />
                                                }.into_any()
                                            } else {
                                                view! { {tmpl_initial} }.into_any()
                                            }}
                                        </div>
                                        <div class="flex flex-col gap-0.5 min-w-0">
                                            <span class="text-sm font-medium text-[var(--content-primary)]">
                                                {tmpl.clone()}
                                            </span>
                                            <span class="text-xs text-[var(--content-secondary)]">
                                                "Template"
                                            </span>
                                        </div>
                                    </div>

                                    // Metadata rows
                                    <dl class="space-y-3 text-sm">
                                        <div class="flex items-center justify-between">
                                            <dt class="text-[var(--content-secondary)]">
                                                "Last used"
                                            </dt>
                                            <dd class="text-[var(--content-primary)] font-medium">
                                                {last_used}
                                            </dd>
                                        </div>
                                        <div class="flex items-center justify-between">
                                            <dt class="text-[var(--content-secondary)]">
                                                "Build"
                                            </dt>
                                            <dd class="text-[var(--content-primary)] font-mono text-xs">
                                                {build_id_short}
                                            </dd>
                                        </div>
                                        {outdated.then(|| view! {
                                            <div class="flex items-center justify-between">
                                                <dt class="text-[var(--content-secondary)]">
                                                    "Version"
                                                </dt>
                                                <dd>
                                                    <span class="inline-flex items-center px-2 py-0.5 text-[11px] font-semibold rounded-full bg-[var(--content-warning)]/15 text-[var(--content-warning)]">
                                                        "Outdated"
                                                    </span>
                                                </dd>
                                            </div>
                                        })}
                                    </dl>
                                </div>
                            </div>

                            // ── Resources (right two columns) ────
                            <div class="lg:col-span-2">
                                <div class="border border-[var(--border-default)] rounded-lg">
                                    <div class="px-4 py-3 border-b border-[var(--border-default)]">
                                        <h2 class="text-sm font-semibold text-[var(--content-secondary)] uppercase tracking-wider">
                                            "Resources"
                                        </h2>
                                    </div>

                                    {if agents.is_empty() {
                                        view! {
                                            <div class="px-4 py-8 text-center text-sm text-[var(--content-secondary)]">
                                                "No agents found for this workspace."
                                            </div>
                                        }.into_any()
                                    } else {
                                        view! {
                                            <div class="divide-y divide-[var(--border-default)]">
                                                {agents.into_iter().map(|(res_name, agent)| {
                                                    view! { <AgentRow resource_name=res_name agent=agent /> }
                                                }).collect::<Vec<_>>()}
                                            </div>
                                        }.into_any()
                                    }}
                                </div>
                            </div>

                        </div>
                    </div>
                }.into_any()
            }
        }}
    }
}

// ─── Loading Skeleton ────────────────────────────────────────────────────────

/// Placeholder skeleton shown while the workspace data is being
/// fetched.  Mirrors the overall page layout so the transition
/// from loading → loaded feels smooth.
#[component]
fn WorkspacePageSkeleton() -> impl IntoView {
    let shimmer = "bg-[var(--surface-tertiary)] rounded animate-pulse";
    let shimmer_lg =
        "bg-[var(--surface-tertiary)] rounded-lg animate-pulse";

    view! {
        // Topbar skeleton
        <div class="border-b border-[var(--border-default)] px-6 py-4">
            <div class="max-w-[1380px] mx-auto flex items-center justify-between gap-4">
                <div class="flex items-center gap-4">
                    <div class={format!("w-5 h-5 {shimmer}")}></div>
                    <div class="flex flex-col gap-1.5">
                        <div class={format!("w-40 h-6 {shimmer_lg}")}></div>
                        <div class={format!("w-20 h-4 {shimmer_lg}")}></div>
                    </div>
                </div>
                <div class="flex items-center gap-3">
                    <div class={format!("w-24 h-5 {shimmer_lg}")}></div>
                    <div class={format!("w-24 h-9 {shimmer_lg}")}></div>
                </div>
            </div>
        </div>

        // Content skeleton
        <div class="max-w-[1380px] mx-auto px-6 py-6">
            <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
                // Details card skeleton
                <div class="lg:col-span-1">
                    <div class="border border-[var(--border-default)] rounded-lg p-4">
                        <div class={format!("w-16 h-4 mb-4 {shimmer}")}></div>
                        <div class="flex items-center gap-3 mb-4">
                            <div class={format!("w-10 h-10 rounded-md {shimmer}")}></div>
                            <div class="flex flex-col gap-1.5">
                                <div class={format!("w-24 h-4 {shimmer}")}></div>
                                <div class={format!("w-16 h-3 {shimmer}")}></div>
                            </div>
                        </div>
                        {(0..3).map(|_| view! {
                            <div class="flex items-center justify-between py-1.5">
                                <div class={format!("w-20 h-4 {shimmer}")}></div>
                                <div class={format!("w-28 h-4 {shimmer}")}></div>
                            </div>
                        }).collect::<Vec<_>>()}
                    </div>
                </div>

                // Resources skeleton
                <div class="lg:col-span-2">
                    <div class="border border-[var(--border-default)] rounded-lg">
                        <div class="px-4 py-3 border-b border-[var(--border-default)]">
                            <div class={format!("w-20 h-4 {shimmer}")}></div>
                        </div>
                        {(0..2).map(|_| view! {
                            <div class="px-4 py-3 flex items-center gap-3">
                                <div class={format!("w-4 h-4 shrink-0 {shimmer}")}></div>
                                <div class="flex flex-col gap-1.5 flex-1">
                                    <div class={format!("w-32 h-4 {shimmer}")}></div>
                                    <div class={format!("w-48 h-3 {shimmer}")}></div>
                                </div>
                            </div>
                        }).collect::<Vec<_>>()}
                    </div>
                </div>
            </div>
        </div>
    }
}

// ─── Agent Row Component ─────────────────────────────────────────────────────

/// A single agent entry inside the resources panel.  Shows the
/// agent name, connection status, OS/architecture, and any
/// workspace apps.
#[component]
fn AgentRow(resource_name: String, agent: Agent) -> impl IntoView {
    let a_dot = agent_status_dot(&agent.status);
    let a_label = agent_status_label(&agent.status).to_string();
    let a_name = agent.name;

    let os_arch = match (
        agent.operating_system.is_empty(),
        agent.architecture.is_empty(),
    ) {
        (true, true) => String::new(),
        (false, true) => agent.operating_system,
        (true, false) => agent.architecture,
        (false, false) => {
            format!(
                "{} / {}",
                agent.operating_system, agent.architecture
            )
        }
    };

    let apps = agent.apps;
    let has_apps = !apps.is_empty();

    view! {
        <div class="px-4 py-3">
            // Agent header row
            <div class="flex items-center gap-3">
                <div class="shrink-0 text-[var(--content-secondary)]">
                    <TerminalIcon />
                </div>
                <div class="flex flex-col gap-0.5 min-w-0 flex-1">
                    <div class="flex items-center gap-2">
                        <span class="text-sm font-medium text-[var(--content-primary)]">
                            {a_name}
                        </span>
                        <span class="text-xs text-[var(--content-secondary)]">
                            {resource_name}
                        </span>
                    </div>
                    <div class="flex items-center gap-3 text-xs text-[var(--content-secondary)]">
                        <span class="inline-flex items-center gap-1">
                            <span class={format!(
                                "w-1.5 h-1.5 rounded-full {}",
                                a_dot
                            )}></span>
                            {a_label}
                        </span>
                        {(!os_arch.is_empty()).then(|| view! {
                            <span class="text-[var(--content-secondary)]">
                                {os_arch}
                            </span>
                        })}
                    </div>
                </div>
            </div>

            // Workspace app tags
            {has_apps.then(|| {
                let app_views: Vec<_> = apps
                    .into_iter()
                    .map(|app| {
                        let label = if app.display_name.is_empty() {
                            "App".to_string()
                        } else {
                            app.display_name
                        };
                        let has_url = !app.url.is_empty();
                        let url = app.url;
                        let has_icon = !app.icon.is_empty();
                        let icon = app.icon;

                        if has_url {
                            view! {
                                <a href={url}
                                   target="_blank"
                                   rel="noopener noreferrer"
                                   class="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-md border border-[var(--border-default)] bg-[var(--surface-secondary)] text-[var(--content-primary)] hover:bg-[var(--surface-tertiary)] no-underline transition-colors">
                                    {has_icon.then(|| view! {
                                        <img src={icon} alt="" class="w-3.5 h-3.5" />
                                    })}
                                    {label}
                                </a>
                            }
                            .into_any()
                        } else {
                            view! {
                                <span class="inline-flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-md border border-[var(--border-default)] bg-[var(--surface-secondary)] text-[var(--content-secondary)]">
                                    {has_icon.then(|| view! {
                                        <img src={icon} alt="" class="w-3.5 h-3.5" />
                                    })}
                                    {label}
                                </span>
                            }
                            .into_any()
                        }
                    })
                    .collect();

                view! {
                    <div class="mt-2 ml-7 flex flex-wrap gap-2">
                        {app_views}
                    </div>
                }
            })}
        </div>
    }
}
