use leptos::prelude::*;
use serde::Serialize;
use wasm_bindgen::JsCast;
use web_sys::HtmlInputElement;

use crate::components::icons::CoderIcon;

/// Request body for POST /api/v2/users/login.
#[derive(Clone, Serialize)]
struct LoginRequest {
    email: String,
    password: String,
}

/// POST credentials to the Coder login endpoint.
async fn login(email: &str, password: &str) -> Result<(), String> {
    let window = web_sys::window().unwrap();
    let base = window.location().origin().unwrap_or_default();
    let url = format!("{}/api/v2/users/login", base);

    let body = LoginRequest {
        email: email.to_string(),
        password: password.to_string(),
    };

    let resp = gloo_net::http::Request::post(&url)
        .header("Content-Type", "application/json")
        .body(serde_json::to_string(&body).unwrap())
        .map_err(|e| format!("Request build error: {e}"))?
        .send()
        .await
        .map_err(|e| format!("Network error: {e}"))?;

    if resp.ok() {
        Ok(())
    } else {
        let text = resp.text().await.unwrap_or_default();
        // Try to extract the "message" field from JSON error responses.
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&text) {
            if let Some(msg) = v.get("message").and_then(|m| m.as_str()) {
                return Err(msg.to_string());
            }
        }
        Err(format!("Login failed ({})", resp.status()))
    }
}

/// Sign-in page for returning users.
#[component]
pub fn LoginPage() -> impl IntoView {
    let (email, set_email) = signal(String::new());
    let (password, set_password) = signal(String::new());
    let (loading, set_loading) = signal(false);
    let (error_msg, set_error_msg) = signal(Option::<String>::None);

    let on_submit = move |ev: web_sys::SubmitEvent| {
        ev.prevent_default();

        let email_val = email.get();
        let password_val = password.get();

        set_loading.set(true);
        set_error_msg.set(None);

        leptos::task::spawn_local(async move {
            match login(&email_val, &password_val).await {
                Ok(()) => {
                    if let Some(window) = web_sys::window() {
                        let _ = window.location().set_href("/workspaces");
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
                    <h1>"Sign in to " <strong>"Coder"</strong></h1>
                </div>

                {move || error_msg.get().map(|msg| view! {
                    <div class="mb-4 px-4 py-3 rounded-lg bg-red-950 border border-red-400 text-red-400 text-sm">
                        {msg}
                    </div>
                })}

                <form class="flex flex-col gap-5" on:submit=on_submit>
                    <div class="flex flex-col gap-1.5">
                        <label class="text-[13px] font-medium text-[var(--content-secondary)]" for="login-email">
                            "Email"
                        </label>
                        <input
                            id="login-email"
                            class="w-full px-3 py-2.5 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)] focus:ring-2 focus:ring-[var(--primary)]/15 placeholder:text-[var(--content-disabled)]"
                            type="text"
                            placeholder="user@example.com"
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
                        <label class="text-[13px] font-medium text-[var(--content-secondary)]" for="login-password">
                            "Password"
                        </label>
                        <input
                            id="login-password"
                            class="w-full px-3 py-2.5 text-sm font-[family-name:var(--font-sans)] text-[var(--content-primary)] bg-[var(--surface-primary)] border border-[var(--border-default)] rounded-lg outline-none transition-colors focus:border-[var(--primary)] focus:ring-2 focus:ring-[var(--primary)]/15 placeholder:text-[var(--content-disabled)]"
                            type="password"
                            placeholder="Password"
                            autocomplete="current-password"
                            required
                            prop:value=move || password.get()
                            on:input=move |ev| {
                                let target: HtmlInputElement =
                                    ev.target().unwrap().unchecked_into();
                                set_password.set(target.value());
                            }
                        />
                    </div>

                    <button
                        type="submit"
                        class="w-full inline-flex items-center justify-center gap-2 rounded-lg text-[15px] font-medium cursor-pointer transition-all border border-transparent px-5 py-3 bg-[var(--content-primary)] text-[var(--content-invert)] hover:bg-gray-300 disabled:opacity-50 disabled:cursor-not-allowed no-underline whitespace-nowrap leading-none"
                        disabled=move || loading.get()
                    >
                        <Show
                            when=move || loading.get()
                            fallback=|| "Sign in"
                        >
                            <span class="inline-block w-4 h-4 border-2 border-black/20 border-t-[var(--content-invert)] rounded-full animate-spin"></span>
                            " Signing in\u{2026}"
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
