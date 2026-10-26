/* coder-kit.js: behavior for the Coder Kit components (../references/components.md). Every component
   renders without it. It adds:
     - tabs: a click or an arrow key on a .ck-tab or .ck-segment selects it and shows its aria-controls panel;
     - dialogs: data-ck-open="<dialog id>" opens a <dialog>, data-ck-close closes the dialog it is in;
     - menus: data-ck-menu="<menu id>" on a button opens that .ck-menu under the button and focuses its first
       item. Arrow keys, Home and End move between items. A click outside, a click on an item, Escape, a scroll or
       a resize closes it. Buttons can share a menu: CoderKit.menuButton(menu) is the button that opened it last;
     - CoderKit.toast(title, { body, tone, timeout }) shows a toast at the bottom right.
   It also works inside shadow roots (html-plan mockups), because it reads event.composedPath(). */
(() => {
  "use strict";
  if (window.CoderKit) return;
  const TAB = ".ck-tab, .ck-segment";
  const TONES = { success: "circle-check", warning: "triangle-alert", error: "circle-x", info: "info" };
  const openMenus = new Map(); // menu element -> the button that opened it
  const lastButton = new WeakMap(); // menu element -> the button that opened it last, also after it closes

  function selectTab(tab) {
    if (!(tab instanceof Element) || !tab.matches(TAB)) throw new TypeError("selectTab needs a .ck-tab or .ck-segment element");
    const list = tab.parentElement;
    for (const t of list.querySelectorAll(":scope > .ck-tab, :scope > .ck-segment")) {
      const on = t === tab;
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
      const id = t.getAttribute("aria-controls");
      const panel = id && t.getRootNode().getElementById(id);
      if (id && !panel) console.warn(`coder-kit: tab panel #${id} not found`);
      if (panel) panel.hidden = !on;
    }
  }

  function toast(title, { body = "", tone = "success", timeout = 5000 } = {}) {
    if (!(tone in TONES)) throw new RangeError(`toast tone must be one of ${Object.keys(TONES).join(", ")}`);
    let stack = document.querySelector(".ck-toast-stack");
    if (!stack) {
      stack = document.createElement("div");
      stack.className = "ck-toast-stack";
      stack.setAttribute("role", "status");
      stack.setAttribute("aria-live", "polite");
      document.body.append(stack);
    }
    const el = document.createElement("div");
    el.className = `ck-toast ck-toast-${tone}`;
    const icon = document.createElement("i");
    icon.className = `ck-icon ck-icon-${TONES[tone]}`;
    icon.setAttribute("aria-hidden", "true");
    const text = document.createElement("div");
    const t = document.createElement("div");
    t.className = "ck-toast-title";
    t.textContent = title;
    text.append(t);
    if (body) {
      const b = document.createElement("div");
      b.className = "ck-toast-body";
      b.textContent = body;
      text.append(b);
    }
    el.append(icon, text);
    stack.append(el);
    if (timeout > 0) setTimeout(() => el.remove(), timeout);
    return el;
  }

  function closeMenu(menu) {
    menu.hidden = true;
    openMenus.get(menu)?.setAttribute("aria-expanded", "false");
    openMenus.delete(menu);
  }

  // The menu is position: fixed while open, so a scrolling table or panel cannot clip it.
  function toggleMenu(button) {
    const id = button.getAttribute("data-ck-menu");
    const menu = button.getRootNode().getElementById(id);
    if (!menu || !menu.classList.contains("ck-menu")) throw new Error(`data-ck-menu="${id}" needs a .ck-menu with id="${id}"`);
    const wasOpenHere = openMenus.get(menu) === button;
    for (const m of [...openMenus.keys()]) closeMenu(m);
    if (wasOpenHere) return;
    // Measure the button first, and take the menu out of the layout before it shows: a menu that shows in
    // place for a moment widens its table cell or bar and moves the button.
    const r = button.getBoundingClientRect();
    Object.assign(menu.style, { position: "fixed", top: "0px", left: "0px", right: "auto" });
    menu.hidden = false;
    const below = r.bottom + 4, above = r.top - 4 - menu.offsetHeight;
    const top = below + menu.offsetHeight > innerHeight && above >= 0 ? above : below;
    const left = Math.min(Math.max(8, r.right - menu.offsetWidth), innerWidth - menu.offsetWidth - 8);
    Object.assign(menu.style, { top: `${top}px`, left: `${left}px` });
    button.setAttribute("aria-expanded", "true");
    openMenus.set(menu, button);
    lastButton.set(menu, button);
    // preventScroll: a scroll closes the menu.
    menu.querySelector(".ck-menu-item")?.focus({ preventScroll: true });
  }

  function menuButton(menu) {
    if (!(menu instanceof Element) || !menu.classList.contains("ck-menu")) throw new TypeError("menuButton needs a .ck-menu element");
    return lastButton.get(menu) ?? null;
  }

  const closeAllMenus = () => { for (const m of [...openMenus.keys()]) closeMenu(m); };
  addEventListener("resize", closeAllMenus);
  addEventListener("scroll", closeAllMenus, true);

  document.addEventListener("click", (e) => {
    const path = e.composedPath();
    for (const [menu, button] of [...openMenus]) {
      const onItem = path.some((el) => el instanceof Element && el.matches(".ck-menu-item") && menu.contains(el));
      if (path.includes(button) || (!onItem && path.includes(menu))) continue;
      closeMenu(menu);
      if (onItem) button.focus({ preventScroll: true });
    }
    for (const el of path) {
      if (!(el instanceof Element)) continue;
      if (el.matches(TAB)) return selectTab(el);
      if (el.hasAttribute("data-ck-menu")) return toggleMenu(el);
      const open = el.getAttribute("data-ck-open");
      if (open) {
        const dialog = el.getRootNode().getElementById(open);
        if (!(dialog instanceof HTMLDialogElement)) throw new Error(`data-ck-open="${open}" needs a <dialog id="${open}">`);
        return dialog.showModal();
      }
      if (el.hasAttribute("data-ck-close")) return el.closest("dialog")?.close();
    }
  });

  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && openMenus.size) {
      const button = [...openMenus.values()].pop();
      closeAllMenus();
      button.focus({ preventScroll: true });
      return e.preventDefault();
    }
    if (openMenus.size && ["ArrowDown", "ArrowUp", "Home", "End"].includes(e.key)) {
      const menu = [...openMenus.keys()].pop();
      const items = [...menu.querySelectorAll(".ck-menu-item")];
      if (!items.length) return;
      const at = items.indexOf(menu.getRootNode().activeElement);
      const last = items.length - 1;
      const next = { Home: 0, End: last, ArrowDown: at < 0 || at === last ? 0 : at + 1, ArrowUp: at <= 0 ? last : at - 1 }[e.key];
      items[next].focus({ preventScroll: true });
      return e.preventDefault();
    }
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    const tab = e.composedPath().find((el) => el instanceof Element && el.matches(TAB));
    if (!tab) return;
    const tabs = [...tab.parentElement.querySelectorAll(":scope > .ck-tab, :scope > .ck-segment")];
    const next = tabs[(tabs.indexOf(tab) + (e.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length];
    selectTab(next);
    next.focus();
    e.preventDefault();
  });

  window.CoderKit = { selectTab, toast, menuButton };
})();
