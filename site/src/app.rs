use leptos::prelude::*;
use leptos_router::{
    components::{Redirect, Route, Router, Routes},
    path,
};

use crate::components::navbar::Navbar;
use crate::pages::login::LoginPage;
use crate::pages::setup::SetupPage;
use crate::pages::workspaces::WorkspacesPage;

#[component]
pub fn App() -> impl IntoView {
    view! {
        <Router>
            <Routes fallback=|| view! { <p>"Page not found."</p> }>
                <Route path=path!("/") view=|| view! { <Redirect path="/workspaces" /> } />
                <Route path=path!("/setup") view=SetupPage />
                <Route path=path!("/login") view=LoginPage />
                <Route
                    path=path!("/workspaces")
                    view=|| view! {
                        <DashboardLayout>
                            <WorkspacesPage />
                        </DashboardLayout>
                    }
                />
                <Route
                    path=path!("/templates")
                    view=|| view! {
                        <DashboardLayout>
                            <p>"Templates coming soon."</p>
                        </DashboardLayout>
                    }
                />
            </Routes>
        </Router>
    }
}

/// Shared layout for authenticated dashboard pages. Verifies the
/// session by calling GET /api/v2/users/me before showing content.
/// Unauthenticated visitors are redirected to /login.
#[component]
fn DashboardLayout(children: Children) -> impl IntoView {
    // None = loading, Some(true) = authed, Some(false) = not authed.
    let (auth_checked, set_auth_checked) = signal(Option::<bool>::None);

    // Check authentication on mount.
    leptos::task::spawn_local(async move {
        let window = web_sys::window().unwrap();
        let base = window.location().origin().unwrap_or_default();
        let url = format!("{}/api/v2/users/me", base);

        let authed = match crate::api::http::get(&url).send().await {
            Ok(resp) => resp.ok(),
            Err(_) => false,
        };
        set_auth_checked.set(Some(authed));
    });

    view! {
        // Centered loading spinner while the auth check is in-flight.
        {move || auth_checked.get().is_none().then(|| view! {
            <div class="flex items-center justify-center min-h-screen">
                <span class="inline-block w-8 h-8 border-4 border-[var(--border-default)] border-t-[var(--content-primary)] rounded-full animate-spin"></span>
            </div>
        })}

        // Redirect to /login when the session is invalid.
        {move || {
            if auth_checked.get() == Some(false) {
                if let Some(w) = web_sys::window() {
                    let _ = w.location().set_href("/login");
                }
            }
        }}

        // Dashboard shell — hidden until authentication succeeds so
        // that children are rendered once and simply revealed.
        <div style=move || {
            if auth_checked.get() == Some(true) {
                "display:contents"
            } else {
                "display:none"
            }
        }>
            <Navbar />
            <main class="main-content">{children()}</main>
        </div>
    }
}
