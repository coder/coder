use leptos::prelude::*;
use leptos_router::hooks::use_location;

use super::icons::CoderIcon;

/// Primary navigation links displayed in the navbar.
const NAV_LINKS: &[(&str, &str)] = &[
    ("Workspaces", "/workspaces"),
    ("Templates", "/templates"),
];

/// Top-level dashboard navigation bar with logo, page links, and
/// a user avatar area.
#[component]
pub fn Navbar() -> impl IntoView {
    let location = use_location();

    view! {
        <nav class="navbar">
            <a href="/workspaces" class="navbar__logo">
                <CoderIcon />
            </a>

            <div class="navbar__nav">
                {NAV_LINKS
                    .iter()
                    .map(|&(label, href)| {
                        let href_owned = href.to_owned();
                        let class = move || {
                            if location.pathname.get().starts_with(&href_owned) {
                                "navbar__link navbar__link--active"
                            } else {
                                "navbar__link"
                            }
                        };
                        view! {
                            <a href=href class=class>
                                {label}
                            </a>
                        }
                    })
                    .collect::<Vec<_>>()}
            </div>

            <div class="navbar__right">
                <div class="navbar__user">
                    <div class="navbar__avatar">"A"</div>
                    <span>"admin"</span>
                </div>
            </div>
        </nav>
    }
}
