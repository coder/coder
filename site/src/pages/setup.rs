use leptos::prelude::*;
use serde::Serialize;
use wasm_bindgen::JsCast;
use web_sys::HtmlInputElement;

use crate::components::icons::CoderIcon;

/// Request body for POST /api/v2/users/first.
#[derive(Clone, Serialize)]
struct CreateFirstUserRequest {
    email: String,
    password: String,
    username: String,
    trial: bool,
}

/// Derive a username from an email address.
fn username_from_email(email: &str) -> String {
    let prefix = email.split('@').next().unwrap_or("admin");
    let username: String = prefix
        .to_lowercase()
        .chars()
        .map(|c| if c.is_ascii_alphanumeric() { c } else { '-' })
        .collect();
    if username.is_empty() {
        "admin".into()
    } else {
        username
    }
}

/// POST the first-user form to the Coder API.
async fn create_first_user(req: CreateFirstUserRequest) -> Result<(), String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!("{}/api/v2/users/first", base);

    let resp = gloo_net::http::Request::post(&url)
        .header("Content-Type", "application/json")
        .body(serde_json::to_string(&req).unwrap())
        .map_err(|e| format!("Request build error: {e}"))?
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if resp.ok() {
        Ok(())
    } else {
        let body = resp.text().await.unwrap_or_default();
        Err(format!("API error ({}): {}", resp.status(), body))
    }
}

/// Login after account creation.
async fn login(email: &str, password: &str) -> Result<(), String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!("{}/api/v2/users/login", base);

    let body = serde_json::json!({
        "email": email,
        "password": password,
    });

    let resp = gloo_net::http::Request::post(&url)
        .header("Content-Type", "application/json")
        .body(body.to_string())
        .map_err(|e| format!("Request build error: {e}"))?
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if resp.ok() {
        Ok(())
    } else {
        let text = resp.text().await.unwrap_or_default();
        Err(format!("Login failed ({}): {}", resp.status(), text))
    }
}

/// Initial account-creation page shown on first launch.
#[component]
pub fn SetupPage() -> impl IntoView {
    let (email, set_email) = signal(String::new());
    let (password, set_password) = signal(String::new());
    let (trial, set_trial) = signal(true);
    let (loading, set_loading) = signal(false);
    let (error_msg, set_error_msg) = signal(Option::<String>::None);

    let on_submit = move |ev: web_sys::SubmitEvent| {
        ev.prevent_default();

        let email_val = email.get();
        let password_val = password.get();
        let trial_val = trial.get();
        let username = username_from_email(&email_val);

        set_loading.set(true);
        set_error_msg.set(None);

        leptos::task::spawn_local(async move {
            let req = CreateFirstUserRequest {
                email: email_val.clone(),
                password: password_val.clone(),
                username,
                trial: trial_val,
            };

            match create_first_user(req).await {
                Ok(()) => {
                    // Auto-login after setup.
                    match login(&email_val, &password_val).await {
                        Ok(()) => {
                            // Redirect to the workspaces page.
                            if let Some(window) = web_sys::window() {
                                let _ = window.location().set_href("/workspaces");
                            }
                        }
                        Err(e) => {
                            set_error_msg.set(Some(e));
                            set_loading.set(false);
                        }
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
        <div class="flex flex-col items-center justify-center min-h-screen p-8">
            <div class="w-full max-w-[385px]">
                <div class="text-center mb-8 [&_svg]:w-12 [&_svg]:h-12 [&_svg]:fill-current [&_svg]:mx-auto [&_h1]:font-normal [&_h1]:text-2xl [&_h1]:mt-4">
                    <CoderIcon />
                    <h1>"Welcome to " <strong>"Coder"</strong></h1>
                    <p class="mt-3 text-[var(--content-secondary)] text-sm">
                        "Let\u{2019}s create your first admin user account"
                    </p>
                </div>

                {move || error_msg.get().map(|msg| view! {
                    <div class="mb-4 px-4 py-3 rounded-lg bg-red-950 border border-red-400 text-red-400 text-sm">
                        {msg}
                    </div>
                })}

                <form class="flex flex-col gap-5" on:submit=on_submit>
                    <div class="flex flex-col gap-1.5">
                        <label class="text-[13px] font-medium text-[var(--content-secondary)]" for="setup-email">"Email"</label>
                        <input
                            id="setup-email"
                            class="w-full px-3 py-2.5 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)] focus:ring-2 focus:ring-[var(--primary)]/15 placeholder:text-[var(--content-disabled)]"
                            type="text"
                            placeholder="admin@example.com"
                            autocomplete="email"
                            required
                            prop:value=move || email.get()
                            on:input=move |ev| {
                                let target: HtmlInputElement =
                                    ev.target().unwrap().unchecked_into();
                                set_email.set(target.value());
                            }
                        />
                    </div>

                    <div class="flex flex-col gap-1.5">
                        <label class="text-[13px] font-medium text-[var(--content-secondary)]" for="setup-password">"Password"</label>
                        <input
                            id="setup-password"
                            class="w-full px-3 py-2.5 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)] focus:ring-2 focus:ring-[var(--primary)]/15 placeholder:text-[var(--content-disabled)]"
                            type="password"
                            placeholder="At least 8 characters"
                            autocomplete="new-password"
                            required
                            prop:value=move || password.get()
                            on:input=move |ev| {
                                let target: HtmlInputElement =
                                    ev.target().unwrap().unchecked_into();
                                set_password.set(target.value());
                            }
                        />
                    </div>

                    <label class="flex items-start gap-2 cursor-pointer text-sm">
                        <input
                            type="checkbox"
                            class="w-[18px] h-[18px] mt-0.5 accent-[var(--primary)] cursor-pointer"
                            prop:checked=move || trial.get()
                            on:change=move |ev| {
                                let target: HtmlInputElement =
                                    ev.target().unwrap().unchecked_into();
                                set_trial.set(target.checked());
                            }
                        />
                        <span>
                            <strong>"Start a free trial of Enterprise"</strong>
                            <span class="block text-[13px] text-[var(--content-secondary)] leading-relaxed">
                                "Get access to high availability, template RBAC, audit logging, and more."
                            </span>
                        </span>
                    </label>

                    <button
                        type="submit"
                        class="w-full inline-flex items-center justify-center gap-2 rounded-lg text-[15px] font-medium cursor-pointer transition-all border border-transparent px-5 py-3 bg-[var(--content-primary)] text-[var(--content-invert)] hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed no-underline whitespace-nowrap leading-none"
                        disabled=move || loading.get()
                    >
                        <Show
                            when=move || loading.get()
                            fallback=|| "Continue with email"
                        >
                            <span class="inline-block w-4 h-4 border-2 border-black/20 border-t-[var(--content-invert)] rounded-full animate-spin"></span>
                            " Creating account\u{2026}"
                        </Show>
                    </button>
                </form>

                <footer class="mt-8 text-center text-xs text-[var(--content-secondary)]">
                    "\u{00a9} 2025 Coder Technologies, Inc."
                </footer>
            </div>
        </div>
    }
}
