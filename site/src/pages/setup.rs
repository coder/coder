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
        <div class="signin-layout">
            <div class="signin-container">
                <div class="signin-header">
                    <CoderIcon />
                    <h1>"Welcome to " <strong>"Coder"</strong></h1>
                    <p class="signin-header__subtitle">
                        "Let\u{2019}s create your first admin user account"
                    </p>
                </div>

                {move || error_msg.get().map(|msg| view! {
                    <div class="alert alert--error" style="margin-bottom:1rem;padding:0.75rem 1rem;border-radius:0.5rem;background:hsl(0,75%,15%);border:1px solid hsl(0,91%,71%);color:hsl(0,91%,71%);font-size:0.875rem">
                        {msg}
                    </div>
                })}

                <form class="signin-form" on:submit=on_submit>
                    <div class="form-field">
                        <label class="form-label" for="setup-email">"Email"</label>
                        <input
                            id="setup-email"
                            class="form-input"
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

                    <div class="form-field">
                        <label class="form-label" for="setup-password">"Password"</label>
                        <input
                            id="setup-password"
                            class="form-input"
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

                    <label class="checkbox-label">
                        <input
                            type="checkbox"
                            prop:checked=move || trial.get()
                            on:change=move |ev| {
                                let target: HtmlInputElement =
                                    ev.target().unwrap().unchecked_into();
                                set_trial.set(target.checked());
                            }
                        />
                        <span class="checkbox-label__text">
                            <strong>"Start a free trial of Enterprise"</strong>
                            <span class="checkbox-label__desc">
                                "Get access to high availability, template RBAC, audit logging, and more."
                            </span>
                        </span>
                    </label>

                    <button
                        type="submit"
                        class="btn btn--primary btn--lg btn--full"
                        disabled=move || loading.get()
                    >
                        <Show
                            when=move || loading.get()
                            fallback=|| "Continue with email"
                        >
                            <span class="spinner spinner--invert"></span>
                            " Creating account\u{2026}"
                        </Show>
                    </button>
                </form>

                <footer class="signin-footer">
                    "\u{00a9} 2025 Coder Technologies, Inc."
                </footer>
            </div>
        </div>
    }
}
