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

/// Shared layout for authenticated dashboard pages. Renders the top-level
/// navigation bar followed by the page content passed as children.
#[component]
fn DashboardLayout(children: Children) -> impl IntoView {
    view! {
        <Navbar />
        <main class="main-content">{children()}</main>
    }
}
