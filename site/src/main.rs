mod app;
mod components;
mod pages;

use app::App;
use wasm_bindgen::JsCast;

fn main() {
    console_error_panic_hook::set_once();

    let root = leptos::prelude::document()
        .get_element_by_id("root")
        .expect("missing #root element")
        .unchecked_into::<web_sys::HtmlElement>();

    leptos::mount::mount_to(root, App).forget();
}
