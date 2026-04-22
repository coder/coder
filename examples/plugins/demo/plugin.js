// Workspace Monitor — Coder plugin demo.
(function () {
    "use strict";

    var ctx = { token: null, apiUrl: null, workspaceId: null, agentId: null };
    var refreshTimer = null;

    // DOM handles.
    var connDot = document.querySelector("#conn-status .status-dot");
    var connLabel = document.querySelector("#conn-status .status-label");
    var wsBody = document.getElementById("workspace-body");
    var agBody = document.getElementById("agent-body");
    var appsBody = document.getElementById("apps-body");
    var logEntries = document.getElementById("log-entries");

    // --- Helpers -----------------------------------------------------------

    function setConnection(state) {
        connDot.className = "status-dot " + state;
        connLabel.textContent =
            state === "ok" ? "Connected" : state === "error" ? "Error" : "Connecting";
    }

    function timeAgo(iso) {
        if (!iso) return "—";
        var s = Math.floor((Date.now() - new Date(iso).getTime()) / 1000);
        if (s < 60) return s + "s ago";
        if (s < 3600) return Math.floor(s / 60) + "m ago";
        if (s < 86400) return Math.floor(s / 3600) + "h ago";
        return Math.floor(s / 86400) + "d ago";
    }

    function badge(text, color) {
        return '<span class="badge badge-' + color + '">' + esc(text) + "</span>";
    }

    function esc(s) {
        var d = document.createElement("span");
        d.textContent = s || "";
        return d.innerHTML;
    }

    function statusColor(s) {
        s = (s || "").toLowerCase();
        if (["running", "connected", "ready", "healthy", "started"].includes(s))
            return "green";
        if (["starting", "connecting", "initializing", "building", "created"].includes(s))
            return "yellow";
        if (["stopped", "failed", "unhealthy", "disconnected", "timeout"].includes(s))
            return "red";
        return "gray";
    }

    function errorBlock(msg) {
        return '<div class="fetch-error">' + esc(msg) + "</div>";
    }

    function markRefreshing(el) { el.classList.add("refreshing"); }
    function markDone(el)       { el.classList.remove("refreshing"); el.classList.remove("placeholder"); }

    // --- Logging -----------------------------------------------------------

    function addLog(tag, msg) {
        var row = document.createElement("div");
        row.className = "log-row";
        var t = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
        row.innerHTML =
            '<span class="log-time">' + t + "</span>" +
            '<span class="log-tag ' + tag + '">' + tag + "</span>" +
            '<span class="log-msg">' + esc(msg) + "</span>";
        logEntries.prepend(row);
        // Keep at most 5 entries.
        while (logEntries.children.length > 5) {
            logEntries.removeChild(logEntries.lastChild);
        }
    }

    // --- API fetching ------------------------------------------------------

    function apiFetch(path) {
        return fetch(ctx.apiUrl + path, {
            headers: { "Coder-Session-Token": ctx.token },
        }).then(function (r) {
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.json();
        });
    }

    function renderWorkspace(data) {
        var owner = data.owner_name || (data.owner && data.owner.username) || "—";
        var status = data.latest_build && data.latest_build.status || "unknown";
        var template = data.template_display_name || data.template_name || "—";
        var lastUsed = data.last_used_at;

        wsBody.innerHTML =
            '<div class="info-grid">' +
                '<span class="info-label">Name</span>'     + '<span class="info-value">' + esc(data.name) + "</span>" +
                '<span class="info-label">Owner</span>'    + '<span class="info-value">' + esc(owner) + "</span>" +
                '<span class="info-label">Status</span>'   + '<span class="info-value">' + badge(status, statusColor(status)) + "</span>" +
                '<span class="info-label">Template</span>' + '<span class="info-value">' + esc(template) + "</span>" +
                '<span class="info-label">Last used</span>' + '<span class="info-value">' + esc(timeAgo(lastUsed)) + "</span>" +
            "</div>";
    }

    function renderAgent(data) {
        var lifecycle = data.lifecycle_state || "—";
        var os = (data.operating_system || "—") + "/" + (data.architecture || "—");
        var ver = data.version || "—";
        if (ver.length > 20) ver = ver.substring(0, 20) + "…";

        agBody.innerHTML =
            '<div class="info-grid">' +
                '<span class="info-label">Name</span>'      + '<span class="info-value">' + esc(data.name || "agent") + "</span>" +
                '<span class="info-label">Status</span>'    + '<span class="info-value">' + badge(data.status || "unknown", statusColor(data.status)) + "</span>" +
                '<span class="info-label">OS / Arch</span>' + '<span class="info-value mono">' + esc(os) + "</span>" +
                '<span class="info-label">Lifecycle</span>' + '<span class="info-value">' + badge(lifecycle, statusColor(lifecycle)) + "</span>" +
                '<span class="info-label">Version</span>'   + '<span class="info-value mono">' + esc(ver) + "</span>" +
            "</div>";

        // Render apps if present.
        var apps = data.apps || [];
        if (apps.length === 0) {
            appsBody.innerHTML = '<span class="muted">No apps registered</span>';
            appsBody.classList.remove("placeholder");
            return;
        }
        var html = '<div class="apps-list">';
        for (var i = 0; i < apps.length; i++) {
            var a = apps[i];
            var h = (a.health || "disabled").toLowerCase();
            html +=
                '<div class="app-row">' +
                    '<span class="app-dot ' + esc(h) + '"></span>' +
                    '<span class="app-name">' + esc(a.display_name || a.slug || "app") + "</span>" +
                    '<span class="app-health">' + esc(h) + "</span>" +
                "</div>";
        }
        html += "</div>";
        appsBody.innerHTML = html;
        appsBody.classList.remove("placeholder");
    }

    function refreshData() {
        if (!ctx.token || !ctx.apiUrl) return;

        markRefreshing(wsBody);
        markRefreshing(agBody);

        var wsOk = false;
        apiFetch("/api/v2/workspaces/" + ctx.workspaceId)
            .then(function (d) { renderWorkspace(d); wsOk = true; })
            .catch(function ()  { wsBody.innerHTML = errorBlock("Unable to fetch workspace"); })
            .finally(function () { markDone(wsBody); });

        apiFetch("/api/v2/workspaceagents/" + ctx.agentId)
            .then(function (d) { renderAgent(d); })
            .catch(function ()  {
                agBody.innerHTML = errorBlock("Unable to fetch agent");
                appsBody.innerHTML = errorBlock("Unable to fetch apps");
                markDone(appsBody);
            })
            .finally(function () { markDone(agBody); });

        addLog("info", "Refreshed workspace & agent data");
    }

    function startPolling() {
        if (refreshTimer) clearInterval(refreshTimer);
        refreshData();
        refreshTimer = setInterval(refreshData, 15000);
    }

    // --- Message handling --------------------------------------------------

    window.addEventListener("message", function (event) {
        var msg = event.data;
        if (!msg || !msg.type || !msg.type.startsWith("coder-plugin:")) return;

        switch (msg.type) {
            case "coder-plugin:init":
                ctx.token = msg.payload.pluginToken;
                ctx.apiUrl = msg.payload.apiUrl;
                ctx.workspaceId = msg.payload.workspaceId;
                ctx.agentId = msg.payload.agentId;

                setConnection("ok");
                window.parent.postMessage({ type: "coder-plugin:ready" }, "*");
                addLog("recv", "Init received — plugin ready");
                startPolling();
                break;

            case "coder-plugin:token-refresh":
                ctx.token = msg.payload.pluginToken;
                addLog("recv", "Token refreshed");
                refreshData();
                break;

            default:
                addLog("recv", msg.type);
        }
    });

    addLog("send", "Waiting for host init…");
})();
