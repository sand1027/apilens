// In-app live-hits overlay. Loaded by a script tag that `apilens init`
// injects into the product app (Stance, or any inited project). Talks to
// the local `apilens ui` process. Does not change frontend or backend URLs.
(function () {
  if (window.__apilensWidgetLoaded) return;
  window.__apilensWidgetLoaded = true;

  var UI = "http://127.0.0.1:4488";
  try {
    if (document.currentScript && document.currentScript.src) {
      UI = new URL(document.currentScript.src).origin;
    }
  } catch (e) {}

  var host = document.createElement("div");
  host.id = "apilens-live-hits";
  host.style.cssText = "all:initial;position:fixed;bottom:16px;right:16px;z-index:2147483647;";
  document.documentElement.appendChild(host);
  var shadow = host.attachShadow({ mode: "open" });

  var events = [];
  var running = false;
  var addr = "";
  var open = false;
  var selected = -1;

  function b64(s) {
    if (!s) return "";
    try {
      return atob(s);
    } catch (e) {
      return "";
    }
  }

  function gqlLabel(ex) {
    var body = b64(ex.Request && ex.Request.Body);
    try {
      var payload = JSON.parse(body);
      var q = (payload.query || "").trim();
      if (q) {
        var method = /^\s*mutation\b/i.test(q)
          ? "MUTATION"
          : /^\s*subscription\b/i.test(q)
            ? "SUBSCRIPTION"
            : "QUERY";
        var named = (payload.operationName || "").trim();
        var field = (q.match(/\{\s*([A-Za-z_][\w]*)/) || [])[1] || "anonymous";
        var gqlError = false;
        try {
          var env = JSON.parse(b64(ex.Response && ex.Response.Body));
          gqlError = Array.isArray(env.errors) && env.errors.length > 0;
        } catch (e2) {}
        return { method: method, name: named || field, gqlError: gqlError };
      }
    } catch (e) {}
    return {
      method: (ex.Request && ex.Request.Method) || "GET",
      name: (ex.Request && ex.Request.URL) || "",
      gqlError: false,
    };
  }

  function ms(ex) {
    var d = ex.Timing && ex.Timing.Duration;
    if (!d) return 0;
    return Math.round(d / 1e6);
  }

  function isOverlayPoll(ex) {
    var raw = (ex.Request && ex.Request.URL) || "";
    try {
      var u = new URL(raw);
      if (u.port !== "4488") return false;
      return (
        u.pathname === "/api/watch/status" ||
        u.pathname.indexOf("/api/watch/") === 0 ||
        u.pathname === "/api/history" ||
        u.pathname.indexOf("/api/history/") === 0 ||
        u.pathname === "/widget.js"
      );
    } catch (e) {
      return raw.indexOf(":4488/") >= 0 && (raw.indexOf("/api/watch") >= 0 || raw.indexOf("/api/history") >= 0);
    }
  }

  function upsert(ex) {
    if (isOverlayPoll(ex)) return;
    var id = ex.ID || String(ex.Display);
    for (var i = 0; i < events.length; i++) {
      if ((events[i].ID || String(events[i].Display)) === id) {
        events[i] = ex;
        return;
      }
    }
    events.unshift(ex);
    if (events.length > 200) events.length = 200;
  }

  function render() {
    var last = events[0];
    var label = last ? gqlLabel(last) : null;
    var lastBit = label
      ? label.method + " " + label.name + " " + (label.gqlError ? "200*" : last.Response.StatusCode) + " " + ms(last) + "ms"
      : "";
    var rows = events
      .map(function (ex, i) {
        var l = gqlLabel(ex);
        var st = l.gqlError ? ex.Response.StatusCode + "*" : String(ex.Response.StatusCode);
        var sel = i === selected ? "background:#1e3a5f;" : "";
        return (
          '<tr data-i="' +
          i +
          '" style="cursor:pointer;border-bottom:1px solid #262626;' +
          sel +
          '">' +
          '<td style="padding:6px 8px;color:#737373;font-family:ui-monospace,monospace;">#' +
          ex.Display +
          "</td>" +
          '<td style="padding:6px 8px;font-family:ui-monospace,monospace;">' +
          l.method +
          "</td>" +
          '<td style="padding:6px 8px;font-family:ui-monospace,monospace;max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">' +
          l.name +
          "</td>" +
          '<td style="padding:6px 8px;font-family:ui-monospace,monospace;">' +
          st +
          "</td>" +
          '<td style="padding:6px 8px;color:#737373;">' +
          ms(ex) +
          "ms</td></tr>"
        );
      })
      .join("");

    var modal = "";
    if (open) {
      var detail = "Select a hit.";
      if (selected >= 0 && events[selected]) {
        var ex = events[selected];
        var l = gqlLabel(ex);
        detail =
          "<div style='font-family:ui-monospace,monospace;font-size:12px;white-space:pre-wrap;word-break:break-all;'>" +
          l.method +
          " " +
          l.name +
          "\n" +
          (ex.Request && ex.Request.URL) +
          "\nstatus " +
          (ex.Response && ex.Response.StatusCode) +
          " · " +
          ms(ex) +
          "ms</div>";
      }
      modal =
        '<div id="apilens-backdrop" style="position:fixed;inset:0;background:rgba(0,0,0,0.55);display:flex;align-items:center;justify-content:center;padding:16px;">' +
        '<div style="width:min(900px,100%);max-height:85vh;background:#0a0a0a;color:#f5f5f5;border:1px solid #404040;border-radius:12px;display:flex;flex-direction:column;overflow:hidden;font-family:system-ui,sans-serif;">' +
        '<div style="padding:12px 16px;border-bottom:1px solid #262626;display:flex;justify-content:space-between;align-items:center;">' +
        "<div><div style='font-weight:600;font-size:14px;'>Live API hits</div>" +
        "<div style='font-size:12px;color:#a3a3a3;'>" +
        (running ? "proxy " + addr : "start apilens ui + apilens watch") +
        " · " +
        events.length +
        " hits</div></div>" +
        '<button type="button" id="apilens-close" style="background:#262626;color:#fff;border:0;border-radius:6px;padding:6px 10px;cursor:pointer;">Close</button></div>' +
        '<div style="display:grid;grid-template-columns:1fr 280px;min-height:240px;overflow:hidden;">' +
        '<div style="overflow:auto;"><table style="width:100%;border-collapse:collapse;font-size:13px;"><tbody>' +
        (rows || '<tr><td style="padding:24px;color:#737373;">No hits yet. Run apilens watch --browser, then use this app.</td></tr>') +
        "</tbody></table></div>" +
        '<div style="border-left:1px solid #262626;padding:12px;overflow:auto;font-size:13px;color:#d4d4d4;">' +
        detail +
        "</div></div></div></div>";
    }

    shadow.innerHTML =
      '<button type="button" id="apilens-chip" style="display:flex;align-items:center;gap:8px;border-radius:9999px;border:1px solid #3b82f6;background:#1d4ed8;color:#fff;padding:8px 14px;cursor:pointer;box-shadow:0 10px 25px rgba(0,0,0,0.45);font:12px/1.3 system-ui,sans-serif;">' +
      '<span style="height:8px;width:8px;border-radius:9999px;background:' +
      (running ? "#86efac" : "#93c5fd") +
      ';display:inline-block;"></span>' +
      "<span>Live hits</span>" +
      '<span style="font-family:ui-monospace,monospace;">' +
      events.length +
      "</span>" +
      (lastBit ? '<span style="font-family:ui-monospace,monospace;max-width:12rem;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">' + lastBit + "</span>" : "") +
      "</button>" +
      modal;

    var chip = shadow.getElementById("apilens-chip");
    if (chip) chip.onclick = function () {
      open = true;
      render();
    };
    var close = shadow.getElementById("apilens-close");
    if (close) close.onclick = function () {
      open = false;
      render();
    };
    var backdrop = shadow.getElementById("apilens-backdrop");
    if (backdrop) {
      backdrop.onclick = function (ev) {
        if (ev.target === backdrop) {
          open = false;
          render();
        }
      };
      var trs = backdrop.querySelectorAll("tr[data-i]");
      for (var t = 0; t < trs.length; t++) {
        trs[t].onclick = function () {
          selected = Number(this.getAttribute("data-i"));
          render();
        };
      }
    }
  }

  async function refresh() {
    try {
      var st = await fetch(UI + "/api/watch/status").then(function (r) {
        return r.json();
      });
      running = !!st.running;
      addr = st.addr || "";
    } catch (e) {
      running = false;
    }
    try {
      var hist = await fetch(UI + "/api/history?limit=200").then(function (r) {
        return r.json();
      });
      if (Array.isArray(hist)) {
        for (var i = 0; i < hist.length; i++) upsert(hist[i]);
      }
    } catch (e) {}
    render();
  }

  try {
    var es = new EventSource(UI + "/api/watch/events");
    es.onmessage = function (evt) {
      try {
        upsert(JSON.parse(evt.data));
        render();
      } catch (e) {}
    };
  } catch (e) {}

  refresh();
  setInterval(refresh, 2000);
  render();
})();
