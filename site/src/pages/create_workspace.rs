//! Create Workspace page — lets the user pick a template and create
//! a new workspace through the Coder API.

use leptos::prelude::*;
use serde::{Deserialize, Serialize};
use wasm_bindgen::JsCast;
use web_sys::HtmlInputElement;

// ─── API Types ───────────────────────────────────────────────────────────────

/// Minimal subset of a Coder template — just enough for the
/// create-workspace form.
#[derive(Clone, Debug, Deserialize, Serialize)]
struct Template {
    id: String,
    name: String,
    #[serde(default)]
    display_name: String,
    #[serde(default)]
    icon: String,
    #[serde(default)]
    organization_name: String,
}

/// Request body for POST /api/v2/users/me/workspaces.
#[derive(Clone, Serialize)]
struct CreateWorkspaceRequest {
    template_id: String,
    name: String,
}

/// Minimal response from workspace creation — just enough to build
/// the redirect URL.
#[derive(Clone, Debug, Deserialize)]
struct WorkspaceCreated {
    #[allow(dead_code)]
    id: String,
    name: String,
    owner_name: String,
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

/// Extract a single query-parameter value from the current URL.
/// Parses `window.location.search` manually so we don't need the
/// `UrlSearchParams` web-sys feature.
fn get_query_param(key: &str) -> Option<String> {
    let search = web_sys::window()?.location().search().ok()?;
    let query = search.strip_prefix('?').unwrap_or(&search);
    for pair in query.split('&') {
        if let Some((k, v)) = pair.split_once('=') {
            if k == key {
                // Percent-decode with js_sys so we handle UTF-8
                // values correctly.
                let decoded = js_sys::decode_uri_component(v)
                    .map(|s| String::from(s))
                    .unwrap_or_else(|_| v.to_string());
                return Some(decoded);
            }
        }
    }
    None
}

// ─── Data Fetching ───────────────────────────────────────────────────────────

async fn fetch_templates() -> Result<Vec<Template>, String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!("{}/api/v2/templates", base);

    let resp = crate::api::http::get(&url)
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if !resp.ok() {
        let text = resp.text().await.unwrap_or_default();
        return Err(format!(
            "Failed to load templates ({}): {}",
            resp.status(),
            text,
        ));
    }

    resp.json::<Vec<Template>>()
        .await
        .map_err(|e| format!("Failed to parse templates: {e}"))
}

async fn create_workspace(
    req: CreateWorkspaceRequest,
) -> Result<WorkspaceCreated, String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!("{}/api/v2/users/me/workspaces", base);

    let resp = crate::api::http::post(&url)
        .body(serde_json::to_string(&req).unwrap())
        .map_err(|e| format!("Request build error: {e}"))?
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if resp.ok() {
        resp.json::<WorkspaceCreated>()
            .await
            .map_err(|e| format!("Failed to parse response: {e}"))
    } else {
        let text = resp.text().await.unwrap_or_default();
        // Try to extract the "message" field from JSON error
        // responses so the user sees something actionable.
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&text) {
            if let Some(msg) = v.get("message").and_then(|m| m.as_str()) {
                return Err(msg.to_string());
            }
        }
        Err(format!("Failed to create workspace ({})", resp.status()))
    }
}

// ─── Page Component ──────────────────────────────────────────────────────────

#[component]
pub fn CreateWorkspacePage() -> impl IntoView {
    // Read ?template= from the URL for pre-selection.
    let preselect_template = get_query_param("template").unwrap_or_default();

    // Form state.
    let (templates, set_templates) =
        signal(Option::<Result<Vec<Template>, String>>::None);
    let (selected_template_id, set_selected_template_id) =
        signal(String::new());
    let (workspace_name, set_workspace_name) =
        signal("my-workspace".to_string());
    let (loading, set_loading) = signal(false);
    let (error_msg, set_error_msg) = signal(Option::<String>::None);

    // Fetch templates on mount.
    {
        let preselect = preselect_template.clone();
        leptos::task::spawn_local(async move {
            let result = fetch_templates().await;
            // Pre-select the template matching the query param, or
            // fall back to the first template in the list.
            if let Ok(ref list) = result {
                let matched = list.iter().find(|t| t.name == preselect);
                let fallback = list.first();
                if let Some(t) = matched.or(fallback) {
                    set_selected_template_id.set(t.id.clone());
                }
            }
            set_templates.set(Some(result));
        });
    }

    let on_submit = move |ev: web_sys::SubmitEvent| {
        ev.prevent_default();

        let template_id = selected_template_id.get();
        let name = workspace_name.get();

        if template_id.is_empty() {
            set_error_msg.set(Some("Please select a template.".into()));
            return;
        }
        if name.is_empty() {
            set_error_msg.set(Some(
                "Please enter a workspace name.".into(),
            ));
            return;
        }

        set_loading.set(true);
        set_error_msg.set(None);

        leptos::task::spawn_local(async move {
            let req = CreateWorkspaceRequest {
                template_id,
                name,
            };
            match create_workspace(req).await {
                Ok(ws) => {
                    if let Some(window) = web_sys::window() {
                        let href =
                            format!("/workspace?owner={}&name={}", ws.owner_name, ws.name);
                        let _ = window.location().set_href(&href);
                    }
                }
                Err(e) => {
                    set_error_msg.set(Some(e));
                    set_loading.set(false);
                }
            }
        });
    };

    view! {
        <div class="max-w-[720px] w-full mx-auto px-6 pb-12">

            // ── Back link ──────────────────────────────────────────
            <a
                href="/templates"
                class="inline-flex items-center gap-1 text-sm text-[var(--content-secondary)] hover:text-[var(--content-primary)] no-underline mb-6 mt-6"
            >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16"
                     viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2" stroke-linecap="round"
                     stroke-linejoin="round" aria-hidden="true">
                    <polyline points="15 18 9 12 15 6" />
                </svg>
                "Back"
            </a>

            // ── Page title ─────────────────────────────────────────
            <h1 class="text-2xl font-semibold mb-6">"New workspace"</h1>

            // ── Error alert ────────────────────────────────────────
            {move || error_msg.get().map(|msg| view! {
                <div class="mb-4 px-4 py-3 rounded-lg bg-red-950 border border-red-400 text-red-400 text-sm">
                    {msg}
                </div>
            })}

            // ── Form card (or loading / error state) ───────────────
            {move || match templates.get() {
                // Still loading templates.
                None => view! {
                    <div class="border border-[var(--border-default)] rounded-lg p-6 flex flex-col gap-6">
                        <div class="flex flex-col gap-1.5">
                            <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block"
                                  style="width:6rem;height:0.875rem"></span>
                            <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block"
                                  style="width:100%;height:2.5rem"></span>
                        </div>
                        <div class="flex flex-col gap-1.5">
                            <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block"
                                  style="width:8rem;height:0.875rem"></span>
                            <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block"
                                  style="width:100%;height:2.5rem"></span>
                        </div>
                        <span class="bg-[var(--surface-tertiary)] rounded-lg animate-pulse block"
                              style="width:100%;height:2.75rem"></span>
                    </div>
                }.into_any(),

                // Template fetch failed.
                Some(Err(err)) => view! {
                    <div class="border border-[var(--border-default)] rounded-lg p-6 flex flex-col items-center justify-center py-16 text-center">
                        <p class="text-lg font-semibold mb-2">"Failed to load templates"</p>
                        <p class="text-[var(--content-secondary)] text-sm max-w-[480px]">{err}</p>
                    </div>
                }.into_any(),

                // No templates exist.
                Some(Ok(list)) if list.is_empty() => view! {
                    <div class="border border-[var(--border-default)] rounded-lg p-6 flex flex-col items-center justify-center py-16 text-center">
                        <p class="text-lg font-semibold mb-2">"No templates available"</p>
                        <p class="text-[var(--content-secondary)] text-sm max-w-[480px]">
                            "An administrator needs to create a template before you can create a workspace."
                        </p>
                    </div>
                }.into_any(),

                // Templates loaded — show the form.
                Some(Ok(list)) => {
                    let options = list.iter().map(|t| {
                        let label = if t.display_name.is_empty() {
                            t.name.clone()
                        } else {
                            format!("{} ({})", t.display_name, t.name)
                        };
                        let id = t.id.clone();
                        let selected = t.id == selected_template_id.get_untracked();
                        view! {
                            <option value={id} selected={selected}>
                                {label}
                            </option>
                        }
                    }).collect::<Vec<_>>();

                    view! {
                        <form
                            class="border border-[var(--border-default)] rounded-lg p-6 flex flex-col gap-6"
                            on:submit=on_submit
                        >
                            // ── Template selector ──────────────
                            <div class="flex flex-col gap-1.5">
                                <label
                                    class="text-[13px] font-medium text-[var(--content-secondary)]"
                                    for="create-ws-template"
                                >
                                    "Template"
                                </label>
                                <select
                                    id="create-ws-template"
                                    class="w-full px-3 py-2.5 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)] focus:ring-2 focus:ring-[var(--primary)]/15 appearance-none cursor-pointer"
                                    on:change=move |ev| {
                                        set_selected_template_id.set(
                                            event_target_value(&ev),
                                        );
                                    }
                                >
                                    {options}
                                </select>
                            </div>

                            // ── Workspace name ─────────────────
                            <div class="flex flex-col gap-1.5">
                                <label
                                    class="text-[13px] font-medium text-[var(--content-secondary)]"
                                    for="create-ws-name"
                                >
                                    "Workspace name"
                                </label>
                                <input
                                    id="create-ws-name"
                                    class="w-full px-3 py-2.5 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)] focus:ring-2 focus:ring-[var(--primary)]/15 placeholder:text-[var(--content-disabled)]"
                                    type="text"
                                    placeholder="my-workspace"
                                    autocomplete="off"
                                    required
                                    prop:value=move || workspace_name.get()
                                    on:input=move |ev| {
                                        let target: HtmlInputElement =
                                            ev.target().unwrap().unchecked_into();
                                        set_workspace_name.set(target.value());
                                    }
                                />
                            </div>

                            // ── Submit button ──────────────────
                            <button
                                type="submit"
                                class="w-full inline-flex items-center justify-center gap-2 rounded-lg text-[15px] font-medium cursor-pointer transition-all border border-transparent px-5 py-3 bg-[var(--content-primary)] text-[var(--content-invert)] hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed no-underline whitespace-nowrap leading-none"
                                disabled=move || loading.get()
                            >
                                <Show
                                    when=move || loading.get()
                                    fallback=|| "Create workspace"
                                >
                                    <span class="inline-block w-4 h-4 border-2 border-black/20 border-t-[var(--content-invert)] rounded-full animate-spin"></span>
                                    " Creating\u{2026}"
                                </Show>
                            </button>
                        </form>
                    }.into_any()
                }
            }}
        </div>
    }
}
