// RepoDash for the Herdr desktop app: every git repo under RepoDash's root as cards.
// Double-click (or Enter) jumps to the repo's space, or makes one, on the machine the
// repos are on.
//
// Where it shows:
//   - as a dialog: `mod.open:repodash` (mod.json's overlay), for any layout;
//   - in a layout of your own, as the `dash` child of a stack with id `view` whose other
//     child, `main`, holds the tabs (`ctrl-shift-r` style toggling between the two).
//
// One mod, several copies (props.part):
//   "dash"   the cards and the inspector (the default)
//   "nav"    a sidebar row that flips the `view` stack; it also shows the terminals again when
//            someone picks a space or tab elsewhere while the cards are up
//   "look"   the Appearance popup (cover style, badge, card sections, size), shared("repodash.look")
//   "gitbar" the title bar of the lazygit dialog
//
// Data: the RepoDash web server (`repodash -web`, config.json's `url`, default
// http://127.0.0.1:8092): POST /api/refresh then GET /api/repos while the cards show;
// /api/config for the card settings; /api/commits, /api/commit, /api/info for the inspector.
// Issues from RepoDash's cache on the repos' machine. When nothing answers, the mod starts
// `<bin> -web -no-open` there once, through herdr-core (config.json's `bin`, written by
// `repodash -install-desktop`). Agents come from this app (agents(), panes()), matched to
// repos by folder. Covers are downloaded once per version into ./.covers (image() reads
// only app files).
//
// config.json (optional; written by `repodash -install-desktop`, yours to edit):
//   url      the server, default http://127.0.0.1:8092
//   bin      absolute path of the repodash binary on the repos' machine
//   args     extra arguments for `bin -web` (e.g. ["-root", "/srv/code"])
//   machine  the Herdr machine the repos are on: "" for this computer, a name from
//            `hc machine ls`; absent: found by looking for `bin` (Windows: the WSL machine)

import { View, div, image, svg } from "gpui-kit";
import { v_flex, h_flex, ScrollHandle, TextView, EditorState, Editor } from "gpui-base";
import { props, run, exec, hc, spaces, panes, agents, machines, focused, layout, theme, settings, appHome, readTextFile, nextCommand, openOverlay, closeOverlay, shared, setShared, notify, window as appWindow } from "herdr";
import * as fs from "fs/promises";
import { hash, nameCover, LANG_COLOR, mixHex, artPath, STYLES } from "./art.js";
import { ago, fmtTok, under, recent, find } from "./util.js";
import { renderInspector } from "./inspector.js";

const DEFAULT_URL = "http://127.0.0.1:8092";
const MONO = "Geist Mono"; // ships with Herdr on every platform
const STACK = "view", DASH = "dash", TERMS = "main";
const GAP = 16, PAD = 18;
const SIZES = { s: 240, m: 280, l: 340 };
const SORTS = [["recent", "Recent"], ["activity", "Activity"], ["language", "Language"], ["alpha", "A–Z"], ["issues", "Issues"]];
const RANGES = [["today", "Today", "d"], ["yesterday", "Yesterday", "D"], ["week", "Week", "w"], ["month", "Month", "m"]];
const LEVELS = ["bad", "warn", "ok", "stale"];
// RepoDash's default status colours (internal/core/default-config.json): [light, dark].
const STATUS = {
  bad: { label: "Needs action", c: ["#c8362b", "#ff6b5e"] },
  warn: { label: "Loose ends", c: ["#b0731a", "#e8b04a"] },
  ok: { label: "Clean", c: ["#2b8a57", "#4cc38a"] },
  stale: { label: "Quiet", c: ["#8a8f99", "#697080"] },
};
const RANK = { blocked: 4, done: 3, working: 2, idle: 1 };
const SPIN = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
const CLEAR = "#00000000";

// What a card shows. RepoDash's `card.*` settings, then this app's Appearance choices on top.
const LOOK_DEFAULTS = { cover: true, coverStyle: "name", languageBadge: true, branch: true, note: true, chips: true, activity: true, issues: true, week: true, agent: true, worktrees: true };
const SECTIONS = [["branch", "Branch"], ["note", "Note"], ["chips", "Status chips"], ["activity", "Last commit"], ["issues", "Next issue"], ["week", "Commits this week"], ["agent", "Agent"], ["worktrees", "Worktree count"]];
function lookOf() {
  const card = shared("repodash.card") ?? {};
  const base = {};
  for (const k of Object.keys(LOOK_DEFAULTS)) if (card[k] !== undefined) base[k] = card[k];
  return { ...LOOK_DEFAULTS, ...base, ...(shared("repodash.look") ?? {}) };
}
// Card width in px, set with the Appearance slider (older choices were s/m/l).
const CARD_W = { min: 180, max: 520, step: 10, def: 280 };
function cardWOf(look) {
  const w = typeof look.cardW === "number" ? look.cardW : SIZES[look.size] ?? CARD_W.def;
  return Math.max(CARD_W.min, Math.min(CARD_W.max, Math.round(w)));
}
function setLook(k, val) { setShared("repodash.look", { ...(shared("repodash.look") ?? {}), [k]: val }); }

const hasStack = () => !!find(layout(), STACK);
const viewShown = () => find(layout(), STACK)?.active ?? DASH;
const showTerminals = () => run("layout.select", { stack: STACK, child: TERMS });
const showCards = () => run("layout.select", { stack: STACK, child: DASH });
const platform = () => appWindow()?.platform ?? "linux";

// Links open on this computer, with its own opener.
function openUrl(url) {
  if (!/^https?:\/\//.test(url ?? "")) return;
  const os = platform();
  const [cmd, args] = os === "windows" ? ["rundll32", ["url.dll,FileProtocolHandler", url]] : os === "macos" ? ["open", [url]] : ["xdg-open", [url]];
  exec(cmd, args).catch(() => {});
}

// File extension from the Content-Type RepoDash serves the cover with.
function imageExt(type) {
  const m = /image\/(png|jpeg|gif|webp|svg\+xml)/.exec(type.toLowerCase());
  return m ? { png: "png", jpeg: "jpg", gif: "gif", webp: "webp", "svg+xml": "svg" }[m[1]] : null;
}

// Pixel size from an image's first bytes (PNG, GIF, JPEG, WebP); null when unknown (SVG).
function imageDims(b) {
  const u16be = (i) => (b[i] << 8) | b[i + 1], u16le = (i) => b[i] | (b[i + 1] << 8);
  const u32be = (i) => ((b[i] << 24) >>> 0) + (b[i + 1] << 16) + (b[i + 2] << 8) + b[i + 3];
  const u24le = (i) => b[i] | (b[i + 1] << 8) | (b[i + 2] << 16);
  if (b.length > 24 && b[0] === 0x89 && b[1] === 0x50) return { w: u32be(16), h: u32be(20) };
  if (b.length > 10 && b[0] === 0x47 && b[1] === 0x49) return { w: u16le(6), h: u16le(8) };
  if (b.length > 30 && b[0] === 0x52 && b[8] === 0x57) { // RIFF....WEBP
    const tag = String.fromCharCode(b[12], b[13], b[14], b[15]);
    if (tag === "VP8 ") return { w: u16le(26) & 0x3fff, h: u16le(28) & 0x3fff };
    if (tag === "VP8L") return { w: 1 + (((b[22] & 0x3f) << 8) | b[21]), h: 1 + (((b[24] & 0x0f) << 10) | (b[23] << 2) | ((b[22] & 0xc0) >> 6)) };
    if (tag === "VP8X") return { w: 1 + u24le(24), h: 1 + u24le(27) };
  }
  if (b.length > 4 && b[0] === 0xff && b[1] === 0xd8) {
    for (let i = 2; i + 9 < b.length;) {
      if (b[i] !== 0xff) { i++; continue; }
      const m = b[i + 1];
      if (m >= 0xc0 && m <= 0xcf && m !== 0xc4 && m !== 0xc8 && m !== 0xcc) return { h: u16be(i + 5), w: u16be(i + 7) };
      i += 2 + u16be(i + 2);
    }
  }
  return null;
}

async function readConfig() {
  try { const c = JSON.parse((await readTextFile(`${appHome()}/mods/repodash/config.json`)) ?? "{}"); return c && typeof c === "object" ? c : {}; } catch { return {}; }
}

// The Herdr machine the repos are on: config.json's, else the one with the binary. On macOS
// and Linux that is this computer; on Windows, RepoDash runs in WSL, a machine of its own.
async function findMachine(cfg) {
  if (typeof cfg.machine === "string") return cfg.machine;
  const has = async (on) => {
    try {
      const probe = cfg.bin ? `test -x '${cfg.bin.replace(/'/g, "'\\''")}'` : "command -v repodash >/dev/null || test -x \"$HOME/.local/bin/repodash\"";
      return (await exec("sh", ["-c", probe], { on: on || undefined, timeout: 8000 })).code === 0;
    } catch { return false; }
  };
  if (platform() !== "windows" && (await has(""))) return "";
  for (const m of machines()) if (m.name && m.layout !== "unavailable" && (await has(m.name))) return m.name;
  return "";
}

export default class RepoDash extends View {
  init(_p, cx) {
    this.part = props()?.part ?? "dash";
    this.base = DEFAULT_URL;
    this.machine = null; // null until known; "" is this computer
    if (this.part === "nav") { this.lastFocus = undefined; this.width = 240; return; }
    if (this.part === "look") { this.dragging = false; return; }
    if (this.part === "notes") { this.initNotes(cx); return; }
    if (this.part === "news") { this.initNews(cx); return; }
    if (this.part !== "dash") return;

    this.repos = null; // null until the first answer
    this.cfg = {};
    this.err = null;
    this.issues = {};
    this.covers = {}; // name -> { src, file, path }
    this.fetching = new Set();
    this.sel = localStorage.getItem("sel");
    this.sort = localStorage.getItem("sort") ?? "recent";
    this.reverse = localStorage.getItem("reverse") === "1";
    this.noRemote = false;
    this.hiddenView = false; // X: only the hidden repos, for this session
    this.inspMode = localStorage.getItem("inspMode") === "all" ? "all" : "repo";
    this.workRange = localStorage.getItem("workRange") ?? "today";
    this.work = null; // { range, data, at }
    this.workLoading = null;
    this.ifocus = -1; // inspector line with the keyboard (J/K), -1 none
    this.inspRows = []; // what J/K move over: [{ id, act }]
    this.upd = null; // { version, latest, blocker }
    this.warnings = [];
    this.filter = localStorage.getItem("filter") ?? "all";
    this.insp = localStorage.getItem("insp") !== "0";
    this.inspW = 380;
    this.query = "";
    this.searching = false;
    this.rootW = 1400;
    this.gridW = 1000;
    this.cols = 3;
    this.lastLoad = 0;
    this.wasShown = false;
    this.det = null; // inspector data for one repo: { name, commits, web, notes, at }
    this.detLoading = null;
    this.openCommit = null;
    this.commitDet = {};
    this.copied = null;
    this.h = cx.focus_handle();
    this.scroll = ScrollHandle.new();
    this.openUrl = openUrl;
    this.overlay = props()?.overlay ?? null; // opened as a dialog (mod.open)
    this.cfgFile = {};
    if (this.overlay) cx.timer.after(60, () => this.h.focus());

    // Poll only while the cards show: a rescan runs git in every repo.
    cx.timer.every(2000, (cx) => {
      const shown = !hasStack() || viewShown() === DASH; // a dialog copy only lives while open
      if (shown && (!this.wasShown || Date.now() - this.lastLoad > (this.err && !this.repos ? 3000 : 15000))) this.load(cx);
      this.wasShown = shown;
    });
    cx.spawn(async (cx) => {
      this.cfgFile = await readConfig();
      if (typeof this.cfgFile.url === "string" && /^https?:\/\//.test(this.cfgFile.url)) this.base = this.cfgFile.url.replace(/\/+$/, "");
      this.load(cx);
      this.checkUpdate(cx);
      this.machine = await findMachine(this.cfgFile);
      cx.notify();
    });
    cx.spawn(async (cx) => {
      for (;;) {
        const c = await nextCommand();
        if (c === "focus") this.h.focus();
        else if (c === "back") this.leave();
        else if (c === "refresh") this.load(cx);
        else if (c === "look") this.openLook();
        else if (c === "inspector") this.toggleInspector();
        else if (c === "hide") { const r = this.selRepo(); if (r) this.toggleHidden(r, cx); }
        else if (c === "pin") { const r = this.selRepo(); if (r) this.togglePin(r, cx); }
        else if (c === "notes") { const r = this.selRepo(); if (r) this.openNotes(r); }
        else if (c === "help") this.openHelp();
        else if (c === "all") this.setInspMode(this.insp && this.inspMode === "all" ? "repo" : "all");
        else if (c === "bigger" || c === "smaller") this.resize(c === "bigger" ? 1 : -1);
        else if (c === "git") { const r = (this.repos ?? []).find((x) => x.name === this.sel); if (r) this.openGit(r, r.path, cx); }
        cx.notify();
      }
    });
  }

  load(cx) {
    this.lastLoad = Date.now();
    const gen = (this.loadGen = (this.loadGen ?? 0) + 1); // only the newest load's answer counts
    cx.spawn(async (cx) => {
      try {
        await fetch(`${this.base}/api/refresh`, { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
        const res = await fetch(`${this.base}/api/repos`);
        if (!res.ok) throw new Error(`RepoDash answered ${res.status}`);
        const repos = (await res.json()) ?? [];
        if (gen !== this.loadGen) return; // a newer load (after a hide or pin) is on its way
        this.repos = repos;
        this.err = null;
        const shown = this.repos.filter((r) => !r.hidden), sum = { total: shown.length, bad: shown.filter((r) => r.level === "bad").length };
        if (JSON.stringify(sum) !== JSON.stringify(shared("repodash.summary") ?? null)) setShared("repodash.summary", sum);
        const c = await fetch(`${this.base}/api/config`);
        if (c.ok) {
          const cj = await c.json();
          this.cfg = cj?.config ?? {};
          this.warnings = cj?.warnings ?? [];
          const card = this.cfg.card ?? {};
          if (JSON.stringify(card) !== JSON.stringify(shared("repodash.card") ?? null)) setShared("repodash.card", card);
        }
        if (this.det && Date.now() - this.det.at > 15000) this.det.stale = true; // refetched, shown meanwhile
        if (this.work && Date.now() - this.work.at > 15000) this.work.stale = true;
      } catch (e) {
        this.err = await this.startServer(e);
      }
      cx.notify();
      if (this.machine === null) return;
      try {
        const r = await exec("sh", ["-c", 'cat "${XDG_CACHE_HOME:-$HOME/.cache}/repodash/issues.json"'], { on: this.machine || undefined, timeout: 10000 });
        if (r.code === 0) { this.issues = JSON.parse(r.stdout).repos ?? {}; cx.notify(); }
      } catch {}
      await this.syncCovers(cx);
    });
  }

  // Nothing answered: start `bin -web` on the repos' machine, once (every copy shares the
  // attempt), as a herdr-core terminal with no pane, so it outlives this window. Returns the
  // message to show meanwhile.
  async startServer(e) {
    const why = String(e?.message ?? e);
    const bin = this.cfgFile.bin;
    if (!bin) return `Can't reach RepoDash at ${this.base}. Start it with \`repodash -web\`, or run \`repodash -install-desktop\` so this view can start it. (${why})`;
    if (this.machine === null) return `Can't reach RepoDash at ${this.base}; looking for it…`;
    const last = Number(shared("repodash.started") ?? 0);
    if (Date.now() - last < 60000) return `Starting RepoDash (${bin} -web)…`;
    setShared("repodash.started", Date.now());
    const args = Array.isArray(this.cfgFile.args) ? this.cfgFile.args.map(String) : [];
    try {
      const on = this.machine ? ["--on", this.machine] : [];
      const r = await hc(["run", "--bg", "--json", "--no-parent", ...on, "--", bin, "-web", "-no-open", ...args]);
      if (r.code !== 0) throw new Error((r.stderr || r.stdout || "").trim() || `hc exited ${r.code}`);
      this.lastLoad = 0; // the 2 s timer loads again
      return `Starting RepoDash (${bin} -web)…`;
    } catch (err) {
      return `Can't reach RepoDash at ${this.base}, and starting it failed: ${err?.message ?? err}`;
    }
  }

  async syncCovers(cx) {
    const dir = `${appHome()}/mods/repodash/.covers`;
    try { await fs.mkdir(dir, { recursive: true }); } catch {}
    for (const r of this.repos ?? []) {
      if (!r.image) { delete this.covers[r.name]; continue; }
      if (this.covers[r.name]?.src === r.image || this.fetching.has(r.name)) continue;
      this.fetching.add(r.name);
      try {
        // The type first (RepoDash sends it), then the file straight to its name.
        const url = this.base + r.image, win = platform() === "windows";
        const curl = win ? "curl.exe" : "curl", nul = win ? "NUL" : "/dev/null";
        const head = await exec(curl, ["-s", "-f", "-m", "10", "-o", nul, "-w", "%{content_type}", url], { timeout: 15000 });
        const ext = imageExt(head.stdout ?? "");
        if (head.code !== 0 || !ext) continue;
        const v = hash(r.image).toString(36), file = `${r.name}-${v}.${ext}`;
        const got = await exec(curl, ["-s", "-f", "-m", "10", "-o", `${dir}/${file}`, url], { timeout: 15000 });
        if (got.code !== 0) continue;
        const old = this.covers[r.name]?.file;
        let dims = null;
        try { dims = imageDims(new Uint8Array(await (await fetch(url)).arrayBuffer())); } catch {}
        this.covers[r.name] = { src: r.image, file, path: `mods/repodash/.covers/${file}`, ...(dims && dims.w > 0 && dims.h > 0 ? dims : {}) };
        if (old && old !== file) fs.unlink(`${dir}/${old}`).catch(() => {});
        cx.notify();
      } catch {} finally { this.fetching.delete(r.name); }
    }
  }

  // ---------- inspector data ----------

  ensureDetail(r, cx) {
    if ((this.det?.name === r.name && !this.det.stale) || this.detLoading === r.name) return;
    this.detLoading = r.name;
    if (this.det?.name !== r.name) this.openCommit = null; // a refresh keeps an open commit open
    cx.spawn(async (cx) => {
      const det = { name: r.name, commits: [], web: null, notes: "", at: Date.now() };
      try {
        const [c, i] = await Promise.all([fetch(`${this.base}/api/commits/${encodeURIComponent(r.name)}`), fetch(`${this.base}/api/info/${encodeURIComponent(r.name)}`)]);
        if (c.ok) { const j = await c.json(); det.commits = j.commits ?? []; det.web = j.web ?? null; }
        if (i.ok) { const j = await i.json(); det.notes = j.notes ?? ""; }
      } catch {}
      if (this.detLoading === r.name) { this.det = det; this.detLoading = null; }
      cx.notify();
    });
  }

  toggleCommit(r, h, cx) {
    this.openCommit = this.openCommit === h ? null : h;
    if (this.openCommit && !this.commitDet[h]) {
      cx.spawn(async (cx) => {
        try {
          const res = await fetch(`${this.base}/api/commit/${encodeURIComponent(r.name)}/${h}`);
          if (res.ok) this.commitDet[h] = await res.json();
        } catch {}
        cx.notify();
      });
    }
    cx.notify();
  }

  // ---------- this app's spaces and agents, by repo folder ----------

  dirsOf(r) { return [r.path, ...(r.worktrees ?? []).map((w) => w.path)]; }

  spaceIn(dirs, ctx) {
    const hit = (cwd) => dirs.some((d) => under(cwd, d));
    let best = null;
    for (const s of ctx.spaces) {
      const ok = hit(s.folder) || s.tabs.some((t) => (ctx.panesByTab.get(t.id) ?? []).some((p) => hit(p.cwd)));
      if (ok && (!best || s.active)) best = s;
    }
    return best;
  }

  spaceFor(r, ctx) { return this.spaceIn(this.dirsOf(r), ctx); }

  agentFor(r, ctx) {
    const dirs = this.dirsOf(r);
    let best = null;
    for (const a of ctx.agents) {
      const cwd = ctx.paneById.get(a.pane)?.cwd;
      if (!dirs.some((d) => under(cwd, d))) continue;
      if (!best || (RANK[a.status] ?? 0) > (RANK[best.status] ?? 0)) best = a;
    }
    if (best) return { status: best.status };
    // An agent RepoDash sees in tmux or the herdr TUI.
    const m = { busy: "working", waiting: "blocked", idle: "idle" }[r.claude];
    return m ? { status: m } : null;
  }

  context() {
    const ps = panes(), panesByTab = new Map();
    for (const p of ps) { if (!panesByTab.has(p.tab)) panesByTab.set(p.tab, []); panesByTab.get(p.tab).push(p); }
    return { spaces: spaces(), agents: agents(), paneById: new Map(ps.map((p) => [p.pane, p])), panesByTab };
  }

  // The repo: any space in it or its worktrees.
  open(r) {
    const s = this.spaceFor(r, this.context());
    if (s) run("space.select", { id: s.id });
    else run("space.new", { folder: r.path, machine: this.machine ?? "" });
    this.leave();
  }

  // One folder (a worktree): a space there, else a new one.
  openFolder(folder) {
    const s = this.spaceIn([folder], this.context());
    if (s) run("space.select", { id: s.id });
    else run("space.new", { folder, machine: this.machine ?? "" });
    this.leave();
  }

  // Back to the terminals: close the dialog, or flip the layout's stack.
  leave() {
    if (this.overlay) closeOverlay(this.overlay);
    else if (hasStack()) showTerminals();
  }

  // lazygit in that folder on the WSL machine, in a dialog over the window. Quitting
  // lazygit ends its terminal, which closes the dialog.
  openGit(r, folder, cx) {
    cx.spawn(async (cx) => {
      try {
        const on = this.machine ? ["--on", this.machine] : [];
        const res = await hc(["run", "--bg", "--json", "--no-parent", "--rm", ...on, "--cwd", folder, "--", "lazygit"]);
        if (res.code !== 0) throw new Error((res.stderr || res.stdout || "").trim() || `hc exited ${res.code}`);
        let addr = JSON.parse(res.stdout).address;
        if (!addr) throw new Error("no terminal address");
        if (this.machine && !addr.includes("/")) addr = `${this.machine}/${addr}`;
        openOverlay({
          kind: "dialog", modal: true, width: 0.92, height: 0.9,
          layout: { type: "column", children: [
            { type: "block", size: { px: 36 }, content: { mod: "repodash", props: { part: "gitbar", terminal: addr, title: folder === r.path ? r.name : `${r.name} · ${folder.split("/").pop()}` } } },
            { type: "block", content: { terminal: addr } },
          ] },
        });
      } catch (e) {
        notify({ title: "Couldn't start lazygit", body: String(e?.message ?? e), level: "error" });
      }
      cx.notify();
    });
  }

  // ---------- actions shared with the TUI and the web page ----------

  selRepo() { return (this.repos ?? []).find((x) => x.name === this.sel) ?? null; }

  async put(path, body) {
    const res = await fetch(`${this.base}${path}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    if (!res.ok) throw new Error(`RepoDash answered ${res.status}`);
  }

  // x: hide or unhide (hidden.json, shared with the TUI and the page); the selection moves on.
  toggleHidden(r, cx) {
    const list = this.visible(), i = list.findIndex((x) => x.name === r.name);
    const next = list[i + 1] ?? list[i - 1] ?? null;
    const hide = !r.hidden;
    r.hidden = hide; // at once; the rescan confirms
    if (next) this.select(next.name);
    if (this.hiddenView && !(this.repos ?? []).some((x) => x.hidden)) this.leaveHiddenView();
    cx.spawn(async (cx) => {
      try { await this.put(`/api/hidden/${encodeURIComponent(r.name)}`, { hidden: hide }); }
      catch (e) { r.hidden = !hide; notify({ title: `Couldn't ${hide ? "hide" : "unhide"} ${r.name}`, body: String(e?.message ?? e), level: "error" }); }
      this.load(cx);
    });
    cx.notify();
  }

  // p: pin or unpin (pins.json, shared with the TUI and the page). Pinned repos come first.
  togglePin(r, cx) {
    const pin = !r.pinned;
    r.pinned = pin;
    cx.spawn(async (cx) => {
      try { await this.put(`/api/pins/${encodeURIComponent(r.name)}`, { pinned: pin }); }
      catch (e) { r.pinned = !pin; notify({ title: `Couldn't ${pin ? "pin" : "unpin"} ${r.name}`, body: String(e?.message ?? e), level: "error" }); }
      this.load(cx);
    });
    cx.notify();
  }

  enterHiddenView() { this.prevSel = this.sel; this.hiddenView = true; const h = this.visible(); if (h.length) this.select(h[0].name, 0); }
  leaveHiddenView() { this.hiddenView = false; if (this.prevSel) this.sel = this.prevSel; }

  // The inspector's All tab: work across every repo in a range (GET /api/work).
  setInspMode(m) { this.inspMode = m; localStorage.setItem("inspMode", m); this.ifocus = -1; if (!this.insp) this.toggleInspector(); }
  setRange(r) { this.workRange = r; localStorage.setItem("workRange", r); this.ifocus = -1; }
  ensureWork(cx) {
    const w = this.work;
    if ((w && w.range === this.workRange && !w.stale) || this.workLoading === this.workRange) return;
    const range = this.workRange;
    this.workLoading = range;
    cx.spawn(async (cx) => {
      let data = null;
      try { const res = await fetch(`${this.base}/api/work?range=${range}`); if (res.ok) data = await res.json(); } catch {}
      if (this.workLoading === range) { this.work = { range, data, at: Date.now() }; this.workLoading = null; }
      cx.notify();
    });
  }

  // Update: the server checks once a day (GET /api/update); U or the chip installs it.
  checkUpdate(cx) {
    cx.spawn(async (cx) => {
      try { const res = await fetch(`${this.base}/api/update`); if (res.ok) { this.upd = await res.json(); cx.notify(); } } catch {}
    });
  }
  applyUpdate(cx) {
    const u = this.upd;
    if (!u?.latest) return;
    if (u.blocker) { notify({ title: "RepoDash can't update itself here", body: u.blocker, level: "warning" }); return; }
    cx.spawn(async (cx) => {
      try {
        const res = await fetch(`${this.base}/api/update`, { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
        if (!res.ok) throw new Error((await res.text()).trim() || `RepoDash answered ${res.status}`);
        notify({ title: `Updating RepoDash to ${u.latest}`, body: "It restarts in a moment.", level: "info" });
        this.upd = { ...u, latest: "" };
      } catch (e) { notify({ title: "Update failed", body: String(e?.message ?? e), level: "error" }); }
      cx.timer.after(4000, (cx) => { this.load(cx); this.checkUpdate(cx); });
      cx.notify();
    });
  }

  openNotes(r) {
    openOverlay({ kind: "dialog", modal: true, width: 0.7, height: 0.85, mod: "repodash", props: { part: "notes", repo: r.name, title: this.titleOf(r), base: this.base } });
  }
  // A small menu popup (the folded top bar). The pick comes back through shared state, since
  // the popup is a copy of its own.
  openMenu(menu, items) {
    openOverlay({ kind: "popup", mod: "repodash", props: { part: "menu", menu, items }, width: 240, height: 16 + items.length * 32 });
  }
  applyPick(pk, cx) {
    if (pk.menu === "filter") this.setFilter(pk.id);
    else if (pk.menu === "sort") { if (pk.id === "reverse") { this.reverse = !this.reverse; localStorage.setItem("reverse", this.reverse ? "1" : "0"); } else this.setSort(pk.id); }
    else if (pk.menu === "tools") {
      const act = { refresh: () => this.load(cx), look: () => this.openLook(), insp: () => this.toggleInspector(), web: () => openUrl(this.base), help: () => this.openHelp(),
        noremote: () => { this.noRemote = !this.noRemote; }, hidden: () => (this.hiddenView ? this.leaveHiddenView() : this.enterHiddenView()),
        warnings: () => this.openWarnings(), update: () => this.applyUpdate(cx), news: () => this.openNews() }[pk.id];
      if (act) act();
    }
    cx.notify();
  }

  openHelp() { openOverlay({ kind: "dialog", width: 620, height: 0.85, mod: "repodash", props: { part: "help" } }); }
  openNews() { openOverlay({ kind: "dialog", width: 640, height: 0.85, mod: "repodash", props: { part: "news", base: this.base } }); }
  openWarnings() { openOverlay({ kind: "dialog", width: 560, height: 360, mod: "repodash", props: { part: "warnings", items: this.warnings } }); }

  // ---------- list ----------

  titleOf(r) { return this.cfg?.repos?.[r.name]?.title || r.name; }
  statusLabel(l) { return this.cfg?.statuses?.[l]?.label || STATUS[l].label; }

  visible() {
    const q = this.query.toLowerCase();
    let list = (this.repos ?? []).filter((r) => !!r.hidden === this.hiddenView);
    if (this.filter !== "all" && !this.hiddenView) list = list.filter((r) => r.level === this.filter);
    if (this.noRemote && !this.hiddenView) list = list.filter((r) => !r.has_remote);
    if (q) list = list.filter((r) => r.name.toLowerCase().includes(q) || this.titleOf(r).toLowerCase().includes(q) || (r.branch ?? "").toLowerCase().includes(q) || (r.language ?? "").toLowerCase().includes(q));
    const by = {
      recent: (a, b) => recent(b) - recent(a),
      activity: (a, b) => (b.commits_7d - a.commits_7d) || (recent(b) - recent(a)),
      language: (a, b) => (!a.language - !b.language) || (a.language ?? "").localeCompare(b.language ?? "") || (recent(b) - recent(a)),
      alpha: (a, b) => a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
      issues: (a, b) => ((this.issues[b.name]?.open ?? 0) - (this.issues[a.name]?.open ?? 0)) || (recent(b) - recent(a)),
    }[this.sort] ?? ((a, b) => recent(b) - recent(a));
    const dir = this.reverse ? -1 : 1; // pinned repos stay first either way
    return list.sort((a, b) => (b.pinned - a.pinned) || dir * by(a, b));
  }

  move(list, d) {
    if (!list.length) return;
    let i = list.findIndex((r) => r.name === this.sel);
    if (i < 0) i = 0;
    else i = Math.max(0, Math.min(list.length - 1, i + d));
    this.select(list[i].name, i);
  }

  select(name, index) {
    if (name !== this.sel && this.inspMode !== "all") this.ifocus = -1;
    this.sel = name;
    localStorage.setItem("sel", name);
    if (index != null) this.scroll.scroll_to_item(index);
  }

  toggleInspector() { this.gridMeasured = null; this.insp = !this.insp; localStorage.setItem("insp", this.insp ? "1" : "0"); }

  openLook() {
    openOverlay({ kind: "popup", mod: "repodash", props: { part: "look", sample: this.sel ?? "repodash" }, width: 330, height: 560 });
  }

  key(e, cx) {
    const list = this.visible();
    const k = e.keystroke;
    if (this.searching) {
      if (k === "escape") { this.query = ""; this.searching = false; }
      else if (k === "enter") this.searching = false;
      else if (k === "backspace") this.query = this.query.slice(0, -1);
      else if (e.key_char && !e.modifiers?.control && !e.modifiers?.alt && !e.modifiers?.platform) this.query += e.key_char;
      else return;
      cx.stop_propagation(); cx.notify(); return;
    }
    const cur = list.find((r) => r.name === this.sel);
    const all = this.insp && this.inspMode === "all";
    const rng = RANGES.find(([, , key]) => key === (k.startsWith("shift-") ? k.slice(6).toUpperCase() : k));
    if (all && rng) { this.setRange(rng[0]); cx.stop_propagation(); cx.notify(); return; }
    switch (k) {
      case "j": case "down": this.move(list, this.cols); break;
      case "k": case "up": this.move(list, -this.cols); break;
      case "l": case "right": this.move(list, 1); break;
      case "h": case "left": this.move(list, -1); break;
      case "g": case "home": if (list.length) this.select(list[0].name, 0); break;
      case "shift-g": case "end": if (list.length) this.select(list[list.length - 1].name, list.length - 1); break;
      case "enter": if (this.ifocus >= 0 && this.inspRows[this.ifocus]) this.inspRows[this.ifocus].act(cx); else if (cur) this.open(cur); break;
      case "c": if (cur) this.open(cur); break;
      case "o": if (this.ifocus >= 0 && this.inspRows[this.ifocus]) this.inspRows[this.ifocus].act(cx); else if (cur) this.open(cur); break;
      case "shift-j": if (this.inspRows.length) { if (!this.insp) this.toggleInspector(); this.ifocus = Math.min(this.inspRows.length - 1, this.ifocus + 1); } break;
      case "shift-k": if (this.inspRows.length) this.ifocus = Math.max(-1, this.ifocus - 1); break;
      case "x": if (cur) this.toggleHidden(cur, cx); break;
      case "shift-x": if (this.hiddenView) this.leaveHiddenView(); else if ((this.repos ?? []).some((r) => r.hidden)) this.enterHiddenView(); break;
      case "p": if (cur) this.togglePin(cur, cx); break;
      case "n": if (cur) this.openNotes(cur); break;
      case "t": this.setInspMode(this.insp && this.inspMode === "all" ? "repo" : "all"); break;
      case "1": if (this.insp) this.setInspMode("repo"); break;
      case "2": if (this.insp) this.setInspMode("all"); break;
      case "shift-s": this.reverse = !this.reverse; localStorage.setItem("reverse", this.reverse ? "1" : "0"); break;
      case "shift-f": { const fs_ = ["all", ...LEVELS], i = fs_.indexOf(this.filter); this.setFilter(fs_[(i + fs_.length - 1) % fs_.length]); break; }
      case "ctrl-d": this.move(list, this.cols * 3); break;
      case "ctrl-u": this.move(list, -this.cols * 3); break;
      case "shift-u": this.applyUpdate(cx); break;
      case "?": case "shift-/": this.openHelp(); break;
      case "shift-l": if (cur) this.openGit(cur, cur.path, cx); break;
      case "i": this.toggleInspector(); break;
      case "a": this.openLook(); break;
      case "-": this.resize(-1); break;
      case "=": case "+": case "shift-=": this.resize(1); break;
      case "/": this.searching = true; break;
      case "s": { const i = SORTS.findIndex(([id]) => id === this.sort); this.setSort(SORTS[(i + 1) % SORTS.length][0]); break; }
      case "f": { const all = ["all", ...LEVELS], i = all.indexOf(this.filter); this.setFilter(all[(i + 1) % all.length]); break; }
      case "r": this.load(cx); break;
      case "w": openUrl(this.base); break;
      case "escape": if (this.query) this.query = ""; else if (this.ifocus >= 0) this.ifocus = -1; else if (this.hiddenView) this.leaveHiddenView(); else this.leave(); break;
      default: return;
    }
    cx.stop_propagation();
    cx.notify();
  }

  resize(dir) { setLook("cardW", Math.max(CARD_W.min, Math.min(CARD_W.max, cardWOf(lookOf()) + dir * 20))); }

  setSort(s) { this.sort = s; localStorage.setItem("sort", s); }
  setFilter(f) { this.filter = f; localStorage.setItem("filter", f); }

  // ---------- drawing ----------

  // A dialog or popup of ours closing (lazygit quit, notes saved, help shut): the cards take
  // the keyboard back, so j/k work at once. Herdr calls close() once as the copy goes.
  async close(_cx) {
    if (this.part !== "dash" && this.part !== "nav") setShared("repodash.refocus", Date.now());
  }

  render(cx) {
    if (this.part === "nav") return this.renderNav(cx);
    if (this.part === "look") return this.renderLook(cx);
    if (this.part === "gitbar") return this.renderGitbar(cx);
    if (this.part === "notes") return this.renderNotes(cx);
    if (this.part === "help") return this.renderHelp(cx);
    if (this.part === "news") return this.renderNews(cx);
    if (this.part === "warnings") return this.renderWarnings(cx);
    if (this.part === "menu") return this.renderMenu(cx);
    return this.renderDash(cx);
  }

  // The sidebar tile that flips the layout's stack between the cards and the terminals: an
  // icon tile, the name and a live summary, so the dashboard is easy to find.
  renderNav(cx) {
    const t = theme(), on = viewShown() === DASH;
    // Someone picked a space or a tab (sidebar, keys, a toast): show its terminals.
    const f = focused(), now = f ? `${f.space}|${f.tab}` : "";
    if (this.lastFocus !== undefined && now !== this.lastFocus && on) showTerminals();
    this.lastFocus = now;
    const compact = this.width < 150, sum = shared("repodash.summary");
    const tile = div().size(36).flex_none().rounded(9).flex().items_center().justify_center()
      .bg(on ? t.accent : t.surface).border(1).border_color(on ? t.accent : t.border)
      .child(svg("mods/repodash/icon.svg").size(20).text_color(on ? t.accent_foreground : t.foreground));
    const sub = h_flex().min_w(0).gap(4).text_size(11.5).whitespace_nowrap().overflow_hidden()
      .child(div().flex_none().text_color(t.muted).child(sum ? `${sum.total} repos` : "your repos"))
      .when(!!sum?.bad, (el) => el.child(div().flex_none().text_color(t.dim).child("·")).child(div().truncate().text_color(STATUS.bad.c[1]).child(`${sum.bad} need action`)));
    return h_flex().id("root").size_full().px(compact ? 0 : 8).items_center().justify_center()
      .on_size_change((size, cx) => { this.width = size.width; cx.notify(); })
      .child(h_flex().id("nav").w_full().h(52).px(compact ? 0 : 8).gap(10).rounded(10).items_center().when(compact, (el) => el.justify_center())
        .bg(on ? t.selected : CLEAR).border(1).border_color(on ? t.border : CLEAR).hover((s) => s.bg(on ? t.selected : t.hover))
        .on_click(() => (on ? showTerminals() : showCards()))
        .child(tile)
        .when(!compact, (el) => el.child(v_flex().flex_1().min_w(0).gap(1)
          .child(div().truncate().whitespace_nowrap().text_size(14).font_weight(650).text_color(t.foreground).child("Repositories"))
          .child(sub))));
  }

  renderGitbar(cx) {
    const t = theme(), p = props() ?? {};
    return h_flex().size_full().px(14).gap(10).items_center().bg(t.chrome).border_b(1).border_color(t.border).text_size(13)
      .child(div().font_weight(600).text_color(t.foreground).child("lazygit"))
      .child(div().text_color(t.muted).truncate().child(p.title ?? ""))
      .child(div().flex_1())
      .child(div().text_size(11.5).text_color(t.dim).child("q in lazygit closes it"))
      .child(div().id("x").px(8).py(2).rounded(5).text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground))
        .on_mouse_down("left", (_e, cx) => cx.stop_propagation())
        .on_click(() => { run("terminal.kill", { terminal: p.terminal }); if (p.overlay) closeOverlay(p.overlay); }).child("✕"));
  }

  // The card-width slider, drawn from plain elements (gpui-base's Slider took the window down
  // when it opened in this popup). Press or drag anywhere on the track.
  widthSlider(t, cardW) {
    const pct = (100 * (cardW - CARD_W.min)) / (CARD_W.max - CARD_W.min);
    const setFrom = (e) => {
      if (!e.local_position || !e.bounds?.width) return;
      const f = Math.max(0, Math.min(1, e.local_position.x / e.bounds.width));
      const n = Math.round((CARD_W.min + f * (CARD_W.max - CARD_W.min)) / CARD_W.step) * CARD_W.step;
      if (n !== cardWOf(lookOf())) setLook("cardW", n);
    };
    return div().id("cw-track").flex_1().h(24).relative().cursor_pointer()
      .on_mouse_down("left", (e, cx) => { this.dragging = true; setFrom(e); cx.notify(); })
      .on_mouse_up("left", () => { this.dragging = false; })
      .on_mouse_move((e) => { if (this.dragging) setFrom(e); })
      .child(div().absolute().left(0).right(0).top(10).h(4).rounded(2).bg(t.border))
      .child(div().absolute().left(0).top(10).h(4).rounded(2).bg(t.accent).w(`${pct.toFixed(2)}%`))
      .child(div().absolute().top(4).left(`${pct.toFixed(2)}%`).ml(-8).size(16).rounded(8).bg(t.accent).border(2).border_color(t.popover));
  }

  renderLook(cx) {
    const t = theme(), look = lookOf(), sample = props()?.sample ?? "repodash", cardW = cardWOf(look);
    const row = (id, label, on, flip) => h_flex().id(id).h(28).px(8).gap(10).rounded(6).items_center().text_size(13)
      .hover((s) => s.bg(t.hover)).on_click(() => flip())
      .child(div().size(16).rounded(4).border(1.5).border_color(on ? t.accent : t.input).bg(on ? t.accent : CLEAR)
        .flex().items_center().justify_center().text_size(11).text_color(t.accent_foreground).child(on ? "✓" : ""))
      .child(div().text_color(t.foreground).child(label));
    const head = (label) => div().pt(6).pb(2).px(2).text_size(11).font_weight(600).text_color(t.dim).child(label.toUpperCase());
    const tile = (style) => {
      const on = look.coverStyle === style, nc = nameCover(sample), path = artPath(sample, style, cx);
      const art = style === "name" ? div().size_full().bg(nc.bg).px(5).overflow_hidden().child(div().whitespace_nowrap().text_size(17).font_weight(800).line_height(1).text_color(nc.ink).child(sample))
        : path ? div().size_full().overflow_hidden().relative().child(image(path).absolute().left(0).top(0).w(132).h(44)) : div().size_full().bg(t.surface);
      return v_flex().id(`st-${style}`).w(140).gap(4).p(3).rounded(8).border(2).border_color(on ? t.accent : CLEAR).hover((s) => s.bg(t.hover))
        .on_click(() => setLook("coverStyle", style))
        .child(div().h(44).w_full().rounded(5).overflow_hidden().child(art))
        .child(div().px(2).text_size(12).text_color(on ? t.foreground : t.muted).child(style[0].toUpperCase() + style.slice(1)));
    };
    return v_flex().id("look").size_full().on_mouse_up("left", () => { this.dragging = false; }).p(10).gap(2).overflow_y_scrollbar().bg(t.popover)
      .child(h_flex().px(2).pb(4).items_center().child(div().flex_1().text_size(14).font_weight(650).text_color(t.foreground).child("Appearance"))
        .child(div().id("reset").px(8).py(2).rounded(5).text_size(12).text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground))
          .on_click(() => setShared("repodash.look", null)).child("Reset")))
      .child(head("Cover"))
      .child(h_flex().flex_wrap().gap(4).children(STYLES.map(tile)))
      .child(row("cover", "Show covers", look.cover, () => setLook("cover", !look.cover)))
      .child(row("badge", "Language badge", look.languageBadge, () => setLook("languageBadge", !look.languageBadge)))
      .child(h_flex().items_baseline().child(head("Card size")).child(div().flex_1())
        .child(div().px(2).text_size(12).text_color(t.muted).child(`${cardW}px`)))
      .child(h_flex().px(4).gap(10).items_center()
        .child(div().text_size(11).text_color(t.dim).child("A"))
        .child(this.widthSlider(t, cardW))
        .child(div().text_size(16).text_color(t.dim).child("A")))
      .child(div().px(2).pb(2).text_size(11.5).text_color(t.dim).child("Or press - and = on the cards."))
      .child(head("On the card"))
      .children(SECTIONS.map(([k, label]) => row(`sec-${k}`, label, look[k], () => setLook(k, !look[k]))))
      .child(div().pt(8).px(2).text_size(11.5).text_color(t.dim).child("Starts from RepoDash's card settings; your choices here win."));
  }

  // ---------- notes: the repo's README and your notes (n), as in the web page's info panel ----------

  initNotes(cx) {
    const p = props() ?? {};
    this.repo = p.repo; this.base = p.base ?? DEFAULT_URL;
    this.info = null; this.editing = false; this.saving = false; this.readmeOpen = true; this.err = null;
    this.ed = EditorState.new({ language: "markdown", value: "", line_numbers: false, folding: false, soft_wrap: true, font_family: MONO });
    this.h = cx.focus_handle();
    cx.spawn(async (cx) => {
      try {
        const res = await fetch(`${this.base}/api/info/${encodeURIComponent(this.repo)}`);
        if (!res.ok) throw new Error(`RepoDash answered ${res.status}`);
        this.info = await res.json();
        this.ed.set_value(this.info.notes ?? "");
        if (!this.info.notes) this.editing = true; // no notes yet: straight into writing them
      } catch (e) { this.err = String(e?.message ?? e); }
      cx.notify();
    });
  }

  saveNotes(cx, close) {
    if (this.saving) return;
    this.saving = true;
    const text = this.ed.value();
    cx.spawn(async (cx) => {
      try {
        const res = await fetch(`${this.base}/api/notes/${encodeURIComponent(this.repo)}`, { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ text }) });
        if (!res.ok) throw new Error(`RepoDash answered ${res.status}`);
        this.info = { ...(this.info ?? {}), notes: text };
        this.editing = false; this.err = null;
        if (close) closeOverlay(props()?.overlay);
      } catch (e) { this.err = `Not saved: ${e?.message ?? e}`; }
      this.saving = false;
      cx.notify();
    });
  }

  renderNotes(cx) {
    const t = theme(), p = props() ?? {}, info = this.info;
    const btn = (id, label, primary, onClick) => div().id(id).h(28).px(12).rounded(7).flex().items_center().flex_none().text_size(12.5)
      .border(1).border_color(primary ? t.accent : t.border).bg(primary ? t.accent : CLEAR).text_color(primary ? t.accent_foreground : t.foreground)
      .hover((s) => s.opacity(0.85)).on_click((_e, cx) => { onClick(cx); cx.notify(); }).child(label);
    const head = h_flex().flex_none().px(16).h(48).gap(10).items_center().border_b(1).border_color(t.border)
      .child(div().text_size(15).font_weight(650).text_color(t.foreground).child(p.title ?? this.repo))
      .child(div().text_size(12).text_color(t.dim).child(this.editing ? "ctrl-s saves · esc closes" : "e edits · esc closes"))
      .child(div().flex_1())
      .when(!!this.err, (el) => el.child(div().text_size(12).text_color(t.blocked).child(this.err)))
      .child(this.editing ? btn("save", this.saving ? "Saving…" : "Save", true, (cx) => this.saveNotes(cx, false))
        : btn("edit", "Edit notes", true, () => { this.editing = true; }))
      .child(btn("close", "Close", false, () => closeOverlay(p.overlay)));
    let body;
    if (!info && !this.err) body = div().p(16).text_color(t.muted).child("Loading…");
    else {
      const readme = info?.readme ? v_flex().flex_none().gap(6)
        .child(div().id("rm-toggle").text_size(11).font_weight(600).text_color(t.dim).hover((s) => s.text_color(t.foreground))
          .on_click((_e, cx) => { this.readmeOpen = !this.readmeOpen; cx.notify(); })
          .child(`${this.readmeOpen ? "▾" : "▸"} ${(info.readme_name || "README").toUpperCase()}`))
        .when(this.readmeOpen, (el) => el.child(div().w_full().text_size(13).text_color(t.foreground).child(TextView.markdown(`readme-${this.repo}`, info.readme)))) : null;
      const notes = v_flex().gap(6).flex_1().min_h(0)
        .child(div().text_size(11).font_weight(600).text_color(t.dim).child("NOTES"))
        .child(this.editing ? div().flex_1().min_h(220).w_full().rounded(8).border(1).border_color(t.input).p(8).child(Editor.new(this.ed).size_full())
          : info?.notes ? div().id("notes-view").w_full().text_size(13).text_color(t.foreground).on_click((_e, cx) => { this.editing = true; cx.notify(); }).child(TextView.markdown(`notes-${this.repo}`, info.notes))
            : div().text_size(13).text_color(t.dim).child("No notes yet."));
      body = v_flex().id("notes-scroll").flex_1().min_h(0).overflow_y_scrollbar().p(16).gap(16)
        .when(!!readme, (el) => el.child(readme)).when(!!readme, (el) => el.child(div().h(1).w_full().bg(t.border))).child(notes);
    }
    return v_flex().id("notes").size_full().bg(t.popover).track_focus(this.h)
      .on_key_down((e, cx) => {
        const k = e.keystroke;
        if (k === "ctrl-s" || k === "cmd-s") { this.saveNotes(cx, false); cx.stop_propagation(); }
        else if (k === "e" && !this.editing) { this.editing = true; cx.stop_propagation(); cx.notify(); }
      })
      .child(head).child(body);
  }

  renderHelp(cx) {
    const t = theme();
    const keys = [
      ["Cards", [["h j k l / arrows", "move"], ["g / G", "first / last"], ["ctrl-d / ctrl-u", "half a screen"], ["Enter, c, double-click", "go to the repo's space (makes one if needed)"],
        ["L", "lazygit in the repo"], ["/", "search (Enter keeps, Esc clears)"], ["s / S", "sort / reverse"], ["f / F", "status filter forward / back"],
        ["x", "hide or unhide the repo"], ["X", "only the hidden repos (Esc back)"], ["p", "pin or unpin"], ["n", "README and notes"], ["r", "rescan"],
        ["- / =", "smaller / bigger cards"], ["a", "Appearance"], ["w", "the web page"], ["U", "update RepoDash"], ["Esc", "back to the terminals"]]],
      ["Inspector", [["i", "show or hide"], ["t, 1 / 2", "This repo / All"], ["d D w m", "All: today, yesterday, week, month"], ["J / K", "move over commits, issues, worktrees"], ["o, Enter", "open that line"]]],
    ];
    return v_flex().id("help").size_full().p(18).gap(14).overflow_y_scrollbar().bg(t.popover)
      .child(h_flex().items_center().child(div().flex_1().text_size(15).font_weight(650).text_color(t.foreground).child("RepoDash keys"))
        .child(div().id("hx").px(8).rounded(5).text_color(t.muted).hover((s) => s.bg(t.hover)).on_click(() => closeOverlay(props()?.overlay)).child("✕")))
      .children(keys.map(([title, rows]) => v_flex().gap(3)
        .child(div().pb(2).text_size(11).font_weight(600).text_color(t.dim).child(title.toUpperCase()))
        .children(rows.map(([k, what]) => h_flex().gap(12).text_size(13)
          .child(div().w(190).flex_none().font_family(MONO).text_color(t.accent).child(k)).child(div().text_color(t.foreground).child(what))))));
  }

  initNews(cx) {
    this.news = null;
    const base = props()?.base ?? DEFAULT_URL;
    cx.spawn(async (cx) => {
      try { const res = await fetch(`${base}/api/changelog`); if (res.ok) this.news = await res.json(); } catch {}
      cx.notify();
    });
  }

  renderNews(cx) {
    const t = theme(), n = this.news;
    const md = n ? (n.releases ?? []).map((r) => `## ${r.tag}\n\n${(r.items ?? []).map((i) => `- ${i}`).join("\n")}`).join("\n\n") : "";
    return v_flex().id("news").size_full().bg(t.popover)
      .child(h_flex().flex_none().px(18).h(48).items_center().border_b(1).border_color(t.border)
        .child(div().flex_1().text_size(15).font_weight(650).text_color(t.foreground).child(`What's new${n?.version ? ` in ${n.version}` : ""}`))
        .child(div().id("nx").px(8).rounded(5).text_color(t.muted).hover((s) => s.bg(t.hover)).on_click(() => closeOverlay(props()?.overlay)).child("✕")))
      .child(div().id("news-scroll").flex_1().min_h(0).overflow_y_scrollbar().p(18).text_size(13).text_color(t.foreground)
        .child(n ? TextView.markdown("news", md || "Nothing listed.") : div().text_color(t.muted).child("Loading…")));
  }

  renderMenu(cx) {
    const t = theme(), p = props() ?? {};
    return v_flex().id("menu").size_full().p(8).gap(0).bg(t.popover)
      .children((p.items ?? []).map((it) => h_flex().id(`m-${it.id}`).h(32).px(10).gap(8).rounded(6).items_center().text_size(13)
        .hover((s) => s.bg(t.hover))
        .on_click(() => { setShared("repodash.pick", { menu: p.menu, id: it.id, at: Date.now() }); closeOverlay(p.overlay); })
        .child(div().w(12).flex_none().text_color(t.accent).child(it.on ? "✓" : ""))
        .child(div().flex_1().truncate().text_color(it.color ?? t.foreground).child(it.label))));
  }

  renderWarnings(cx) {
    const t = theme(), items = props()?.items ?? [];
    return v_flex().id("warnings").size_full().p(18).gap(8).overflow_y_scrollbar().bg(t.popover)
      .child(h_flex().items_center().child(div().flex_1().text_size(15).font_weight(650).text_color(t.foreground).child("Problems in RepoDash's settings"))
        .child(div().id("wx").px(8).rounded(5).text_color(t.muted).hover((s) => s.bg(t.hover)).on_click(() => closeOverlay(props()?.overlay)).child("✕")))
      .child(div().text_size(12.5).text_color(t.muted).child("Each one falls back to its default; the rest of the file still applies. Check with `repodash -check`."))
      .children(items.map((w) => div().text_size(13).text_color(t.foreground).child(`· ${w}`)));
  }

  renderDash(cx) {
    const rf = shared("repodash.refocus");
    if (rf && rf !== this.lastRefocus) { this.lastRefocus = rf; this.h.focus(); }
    const pk = shared("repodash.pick");
    if (pk && pk.at !== this.lastPick) {
      const first = this.lastPick === undefined;
      this.lastPick = pk.at;
      if (!first) cx.timer.after(1, (cx) => this.applyPick(pk, cx)); // a pick from before this copy started is old news
    }
    const t = theme(), dark = !!settings().dark, list = this.visible(), ctx = this.context(), look = lookOf();
    const sc = (lvl) => this.cfg?.statuses?.[lvl]?.[dark ? "darkColor" : "color"] || STATUS[lvl].c[dark ? 1 : 0];
    const cardMin = cardWOf(look);
    // Cards are exactly the slider's width, from the top left. The keys need the column count, so the
    // grid's reported width is converted to drawn units with a 100px probe (zoom skews it).
    let cols = 3;
    if (this.gridMeasured && this.probeMeasured) {
      const w = this.gridMeasured * 100 / this.probeMeasured - 2 * PAD;
      cols = Math.max(1, Math.floor((w + GAP) / (cardMin + 4 + GAP)));
    }
    this.cols = cols;
    if (!list.some((r) => r.name === this.sel) && list.length) this.sel = list[0].name;

    const counts = { all: 0 };
    for (const r of this.repos ?? []) { if (r.hidden) continue; counts.all++; counts[r.level] = (counts[r.level] ?? 0) + 1; }

    const btn = (id, active, color, label, onClick) => h_flex().id(id).h(28).px(11).gap(6).rounded(14).items_center().flex_none()
      .text_size(12.5).border(1).border_color(active ? (color ?? t.accent) : (color ? color + "88" : t.border))
      .bg(active ? (color ?? t.accent) : CLEAR)
      .text_color(active ? (color ? "#ffffff" : t.accent_foreground) : (color ?? t.foreground))
      .hover((s) => s.bg(active ? (color ?? t.accent) : t.hover))
      .on_click((_e, cx) => { onClick(); cx.notify(); })
      .children(label);

    const hiddenN = (this.repos ?? []).filter((r) => r.hidden).length;
    const noRemoteN = (this.repos ?? []).filter((r) => !r.hidden && !r.has_remote).length;
    const ags = agents(), nWork = ags.filter((a) => a.status === "working").length, nWait = ags.filter((a) => a.status === "blocked").length;
    const agentSum = [nWork ? `${nWork} working` : "", nWait ? `${nWait} waiting` : ""].filter(Boolean).join(" · ");
    const quiet = (id, label, color, onClick) => div().id(id).h(28).px(10).rounded(14).flex().items_center().flex_none().text_size(12.5)
      .text_color(color ?? t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground)).on_click((_e, cx) => { onClick(cx); cx.notify(); }).child(label);
    const iconBtn = (id, label, on, onClick) => div().id(id).h(28).px(10).rounded(14).flex().items_center().flex_none().text_size(12.5)
      .border(1).border_color(on ? t.accent : t.border).bg(on ? t.selected : CLEAR).text_color(on ? t.foreground : t.muted)
      .hover((s) => s.bg(t.hover).text_color(t.foreground))
      .on_click((_e, cx) => { onClick(cx); cx.notify(); }).child(label);
    const menuBtn = (id, label, color, onClick) => h_flex().id(id).h(28).px(11).gap(6).rounded(14).items_center().flex_none().text_size(12.5)
      .border(1).border_color(color ? color + "88" : t.border).text_color(color ?? t.foreground).hover((s) => s.bg(t.hover))
      .on_click((_e, cx) => { onClick(cx); cx.notify(); }).child(label).child(div().text_color(t.dim).child("▾"));

    // One row, always: as the window narrows the bar folds, in this order: the sort buttons
    // become a menu; the tools (refresh, Appearance, inspector, Web, version) become a ⋯ menu;
    // the status buttons lose their words; they become a menu too. Widths are estimated from
    // the text, generously, in drawn units.
    const uiFont = String(settings().ui_font ?? settings().ui_font_family ?? "");
    const cw = !uiFont || /mono|code|consol|courier|menlo/i.test(uiFont) ? 7.6 : 6.9, txt = (str) => cw * str.length;
    const pillW = (label, count, dot) => 24 + txt(label) + (count != null ? 6 + txt(String(count)) : 0) + (dot ? 13 : 0);
    const statusN = (l) => counts[l] ?? 0;
    const W = {
      title: 10.5 * "Repositories".length + 6,
      filtersFull: pillW("All", counts.all) + LEVELS.reduce((n, l) => n + 6 + pillW(this.statusLabel(l), statusN(l), true), 0),
      filtersDots: pillW("All", counts.all) + LEVELS.reduce((n, l) => n + 6 + pillW("", statusN(l), true), 0),
      filtersMenu: pillW(this.filter === "all" ? "All" : this.statusLabel(this.filter), 99, this.filter !== "all") + 14,
      extras: (noRemoteN ? 10 + pillW("No remote", noRemoteN) : 0) + (hiddenN ? 10 + 20 + txt(`${hiddenN} hidden`) : 0) + (agentSum ? 10 + txt(agentSum) : 0),
      sortsFull: 8 + SORTS.reduce((n, [, label]) => n + 22 + txt(label), 0),
      sortsMenu: 36 + txt(SORTS.find(([id]) => id === this.sort)?.[1] ?? ""),
      tools: 3 * 40 + 10 + (24 + txt("Web")) + (this.warnings.length ? 10 + 22 + txt(`⚠ ${this.warnings.length}`) : 0)
        + (this.upd?.latest ? 10 + 22 + txt(`↑ ${this.upd.latest}`) : 0) + (this.upd?.version ? 10 + 22 + txt(this.upd.version) : 0) + 30,
      toolsMenu: 40,
    };
    // The root's own size comes in drawn units (unlike its children's; see the probe).
    const avail = this.rootW ? this.rootW - 2 * PAD : 1e9;
    const LV = [
      { filt: "full", extras: true, search: 190, sortMenu: false, toolsMenu: false },
      { filt: "full", extras: true, search: 190, sortMenu: true, toolsMenu: false },
      { filt: "full", extras: true, search: 190, sortMenu: true, toolsMenu: true },
      { filt: "dots", extras: true, search: 160, sortMenu: true, toolsMenu: true },
      { filt: "menu", extras: true, search: 150, sortMenu: true, toolsMenu: true },
      { filt: "menu", extras: false, search: 110, sortMenu: true, toolsMenu: true },
    ];
    const need = (lv) => W.title + 10 + { full: W.filtersFull, dots: W.filtersDots, menu: W.filtersMenu }[lv.filt] + (lv.extras ? W.extras : 0)
      + 10 + lv.search + 10 + (lv.sortMenu ? W.sortsMenu : W.sortsFull) + 10 + (lv.toolsMenu ? W.toolsMenu : W.tools);
    const lv = LV.find((x) => need(x) <= avail) ?? LV[LV.length - 1];

    const filterItems = [["all", "All", counts.all, null], ...LEVELS.map((l) => [l, this.statusLabel(l), statusN(l), sc(l)])];
    let filters;
    if (lv.filt === "menu") {
      const [, label, n, col] = filterItems.find(([id]) => id === this.filter) ?? filterItems[0];
      filters = menuBtn("f-menu", `${label} ${n}`, col, () => this.openMenu("filter", filterItems.map(([id, l, c, color]) => ({ id, label: `${l}  ${c}`, on: this.filter === id, color }))));
    } else {
      const words = lv.filt === "full";
      filters = h_flex().gap(6).items_center().flex_none()
        .child(btn("f-all", this.filter === "all", null, [div().child("All"), div().text_color(this.filter === "all" ? t.accent_foreground : t.muted).child(String(counts.all))], () => this.setFilter("all")))
        .children(LEVELS.map((l) => btn(`f-${l}`, this.filter === l, sc(l),
          [div().size(7).rounded(4).bg(this.filter === l ? "#ffffff" : sc(l)), ...(words ? [div().child(this.statusLabel(l))] : []), div().opacity(0.75).child(String(statusN(l)))],
          () => this.setFilter(this.filter === l ? "all" : l))));
    }

    const sorts = lv.sortMenu
      ? menuBtn("s-menu", `${SORTS.find(([id]) => id === this.sort)?.[1] ?? "Sort"}${this.reverse ? " ↓" : ""}`, null, () => this.openMenu("sort", [
        ...SORTS.map(([id, label]) => ({ id, label, on: this.sort === id })), { id: "reverse", label: "Reverse order  S", on: this.reverse }]))
      : h_flex().h(28).p(2).gap(2).rounded(14).border(1).border_color(t.border).items_center().flex_none()
        .children(SORTS.map(([id, label]) => div().id(`s-${id}`).h(22).px(10).rounded(11).flex().items_center().text_size(12.5)
          .bg(this.sort === id ? t.selected : CLEAR).text_color(this.sort === id ? t.foreground : t.muted)
          .hover((s) => s.bg(t.hover)).on_click((_e, cx) => { this.setSort(id); cx.notify(); }).child(label)));

    const search = h_flex().id("search").h(28).w(lv.search).px(10).gap(6).rounded(14).border(1).items_center().flex_none()
      .border_color(this.searching ? t.accent : t.border).text_size(12.5)
      .on_click((_e, cx) => { this.searching = true; this.h.focus(); cx.notify(); })
      .child(div().text_color(t.dim).child("⌕"))
      .child(this.query || this.searching
        ? h_flex().flex_1().min_w(0).child(div().truncate().text_color(t.foreground).child(this.query)).when(this.searching, (el) => el.child(div().w(1.5).h(14).bg(t.accent).loop("opacity", { from: 0, to: 1, period: 1000 })))
        : div().flex_1().min_w(0).truncate().text_color(t.dim).child("Search  /"));

    const toolItems = [
      { id: "refresh", label: "Rescan  r" }, { id: "look", label: "Appearance  a" }, { id: "insp", label: `${this.insp ? "Hide" : "Show"} inspector  i`, on: this.insp },
      { id: "web", label: "Open the web page  w" }, { id: "help", label: "Keys  ?" },
      ...(lv.extras ? [] : [...(noRemoteN ? [{ id: "noremote", label: `No remote only (${noRemoteN})`, on: this.noRemote }] : []), ...(hiddenN ? [{ id: "hidden", label: `Hidden repos (${hiddenN})  X`, on: this.hiddenView }] : [])]),
      ...(this.warnings.length ? [{ id: "warnings", label: `Settings problems (${this.warnings.length})`, color: t.blocked }] : []),
      ...(this.upd?.latest ? [{ id: "update", label: `Update to ${this.upd.latest}  U`, color: sc("warn") }] : []),
      ...(this.upd?.version ? [{ id: "news", label: `What's new in ${this.upd.version}` }] : []),
    ];
    const tools = lv.toolsMenu
      ? [iconBtn("tools", "⋯", false, () => this.openMenu("tools", toolItems))]
      : [iconBtn("refresh", "↻", false, (cx) => this.load(cx)), iconBtn("look", "◐", false, () => this.openLook()),
        iconBtn("inspbtn", "◫", this.insp, () => this.toggleInspector()), iconBtn("web", "Web", false, () => openUrl(this.base)),
        ...(this.warnings.length ? [quiet("warn", `⚠ ${this.warnings.length}`, t.blocked, () => this.openWarnings())] : []),
        ...(this.upd?.latest ? [quiet("upd", `↑ ${this.upd.latest}`, sc("warn"), (cx) => this.applyUpdate(cx))] : []),
        ...(this.upd?.version ? [quiet("ver", this.upd.version, t.dim, () => this.openNews())] : [])];

    const top = h_flex().flex_none().w_full().px(PAD).pt(14).pb(12).gap(10).items_center().overflow_hidden()
      .child(div().flex_none().mr(6).text_size(17).font_weight(700).text_color(t.foreground).whitespace_nowrap().child(this.cfg?.title && this.cfg.title !== "RepoDash" ? this.cfg.title : "Repositories"))
      .child(filters)
      .when(lv.extras && noRemoteN > 0, (el) => el.child(btn("f-remote", this.noRemote, null, [div().child("No remote"), div().opacity(0.75).child(String(noRemoteN))], () => { this.noRemote = !this.noRemote; })))
      .when(lv.extras && hiddenN > 0, (el) => el.child(quiet("hiddenbtn", `${hiddenN} hidden`, this.hiddenView ? t.accent : null, () => (this.hiddenView ? this.leaveHiddenView() : this.enterHiddenView()))))
      .when(lv.extras && !!agentSum, (el) => el.child(div().flex_none().whitespace_nowrap().text_size(12.5).text_color(nWait ? t.blocked : t.working).child(agentSum)))
      .child(div().flex_1().min_w(0))
      .child(search)
      .child(sorts)
      .children(tools);

    let body;
    if (this.repos === null && !this.err) body = div().p(PAD).text_color(t.muted).child("Loading…");
    else if (this.err && !this.repos) body = div().p(PAD).text_color(t.blocked).child(this.err);
    else if (!list.length) body = div().p(PAD).text_color(t.muted).child(this.query ? `Nothing matches “${this.query}”.` : "No repos here.");
    else {
      body = div().id("grid").flex_1().min_h(0).w_full().overflow_y_scrollbar().track_scroll(this.scroll)
        .on_size_change((size, cx) => { if (Math.abs(size.width - (this.gridMeasured ?? 0)) > 1) { this.gridMeasured = size.width; cx.notify(); } })
        .flex().flex_wrap().content_start().gap(GAP).p(PAD)
        .children(list.map((r, i) => this.card(r, i, cardMin, t, sc, ctx, look, cx)));
    }

    const strip = this.hiddenView ? h_flex().flex_none().mx(PAD).mt(10).h(32).px(12).gap(10).rounded(8).items_center().text_size(12.5)
      .bg(sc("warn") + "26").border(1).border_color(sc("warn") + "88").text_color(t.foreground)
      .child(div().font_weight(600).child(`Hidden repos (${hiddenN})`))
      .child(div().text_color(t.muted).child("x unhides · X or Esc goes back"))
      .child(div().flex_1())
      .child(div().id("hv-x").px(6).rounded(5).text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground))
        .on_click((_e, cx) => { this.leaveHiddenView(); cx.notify(); }).child("✕")) : null;
    const left = v_flex().id("left").flex_1().min_w(0).h_full().min_h(0)
      .when(!!strip, (el) => el.child(strip))
      .when(!!(this.err && this.repos), (el) => el.child(div().px(PAD).pt(8).text_size(12).text_color(t.blocked).child(this.err)))
      .child(body);

    const selRepo = (this.repos ?? []).find((r) => r.name === this.sel) ?? null;
    return v_flex().id("root").relative().size_full().min_h(0).bg(t.canvas).rounded(10).overflow_hidden()
      .on_size_change((size, cx) => { if (Math.abs(size.width - this.rootW) > 1) { this.rootW = size.width; cx.notify(); } })
      .child(div().id("probe").absolute().left(0).top(0).w(100).h(0)
        .on_size_change((size, cx) => { if (size.width > 0 && Math.abs(size.width - (this.probeMeasured ?? 0)) > 0.5) { this.probeMeasured = size.width; cx.notify(); } }))
      .track_focus(this.h).on_key_down((e, cx) => this.key(e, cx))
      .on_mouse_down("left", () => this.h.focus())
      .child(top)
      .child(h_flex().flex_1().min_h(0).w_full().border_t(1).border_color(t.border)
        .child(left)
        .when(this.insp && !!this.repos, (el) => el.child(renderInspector(this, selRepo, cx, t, sc, ctx))));
  }

  // One card, drawn at `w` px wide. Everything in it (text, cover, spacing) is sized from the
  // same scale, w / 280, so the size slider zooms the whole card the way thumbnail sliders do.
  card(r, i, w, t, sc, ctx, look, cx) {
    const k = w / CARD_W.def, z = (n) => Math.round(n * k * 10) / 10;
    const selected = r.name === this.sel, color = sc(r.level);
    const rc = this.cfg?.repos?.[r.name] ?? {};
    const coverH = z(this.cfg?.card?.coverHeight ?? 98);
    const innerW = w - 2 - 2 * z(6); // the cover band: the card less its border and padding
    const narrow = w < 250;
    const space = this.spaceFor(r, ctx);
    const agent = look.agent ? this.agentFor(r, ctx) : null;

    const pill = (abs) => h_flex().when(abs, (el) => el.absolute().left(z(8)).bottom(z(8))).h(z(22)).px(z(10)).gap(z(6)).rounded(z(11)).items_center().text_size(z(12)).font_weight(600).flex_none()
      .bg(r.level === "bad" ? color : t.background).text_color(r.level === "bad" ? "#ffffff" : color)
      .when(r.level === "warn", (el) => el.border(1.5).border_color(color))
      .when(!abs && r.level !== "bad" && r.level !== "warn", (el) => el.border(1).border_color(color + "88"))
      .child(div().size(z(7)).rounded(z(4)).bg(r.level === "bad" ? "#ffffff" : color)).child(this.statusLabel(r.level));

    // Cover: the repo's image, else generated art (RepoDash's styles, from the repo name).
    let coverEl = null;
    if (look.cover) {
      const cover = this.covers[r.name], style = rc.coverStyle || look.coverStyle;
      let media;
      if (cover && r.image_fit === "contain") {
        media = div().size_full().p(z(14)).bg(t.surface).child(image(cover.path).size_full());
      } else if (cover && cover.w && cover.h) {
        // Fill the band and crop, as the web page's object-fit: cover does.
        const f = Math.max(innerW / cover.w, coverH / cover.h), iw = cover.w * f, ih = cover.h * f;
        media = div().size_full().relative().overflow_hidden()
          .child(image(cover.path).absolute().left((innerW - iw) / 2).top((coverH - ih) / 2).w(iw).h(ih));
      } else if (cover) {
        media = div().size_full().bg(t.surface).child(image(cover.path).size_full());
      } else if (style === "name") {
        const nc = nameCover(r.name);
        media = div().size_full().bg(nc.bg).px(z(14)).pt(z(6)).overflow_hidden()
          .child(div().whitespace_nowrap().text_size(coverH * 0.44).font_weight(800).line_height(1).text_color(nc.ink).child(r.name));
      } else {
        // The art is 3:1; draw it wide enough to cover the band and let the band crop it.
        const path = artPath(r.name, style, cx), iw = Math.max(innerW, coverH * 3), ih = iw / 3;
        media = path ? div().size_full().relative().overflow_hidden().child(image(path).absolute().left((innerW - iw) / 2).top((coverH - ih) / 2).w(iw).h(ih))
          : div().size_full().bg(nameCover(r.name).bg);
      }
      const lang = r.language && look.languageBadge ? div().absolute().right(z(8)).when(narrow, (el) => el.top(z(8))).when(!narrow, (el) => el.bottom(z(8))).px(z(8)).py(z(4)).rounded(z(7)).text_size(z(11)).font_weight(600)
        .font_family(MONO).text_color("#ffffff").bg(mixHex(LANG_COLOR[r.language] || "#667788", "#000000", 0.38)).child(r.language) : null;
      coverEl = div().relative().h(coverH).w_full().rounded(z(9)).overflow_hidden().bg(t.surface)
        .child(media).child(pill(true)).when(!!lang, (el) => el.child(lang));
    }

    // Header: name, then where it is open and its agent.
    const agentEl = agent ? (() => {
      const col = t[agent.status] ?? t.muted;
      const word = { working: "working", blocked: "waiting", done: "done", idle: "idle" }[agent.status] ?? agent.status;
      const ctxPct = r.context && r.context_size ? r.context / r.context_size : 0;
      const ctxCol = ctxPct >= 0.9 ? sc("bad") : ctxPct >= 0.7 ? sc("warn") : t.muted;
      return h_flex().h(z(20)).px(z(7)).gap(z(5)).rounded(z(10)).border(1).border_color(col + "99").items_center().text_size(z(11)).flex_none()
        .child(agent.status === "working" ? div().text_color(col).frames(SPIN, 90).child(SPIN[0])
          : div().size(z(7)).rounded(z(4)).bg(col).when(agent.status === "blocked", (el) => el.loop("opacity", { from: 0.35, to: 1, period: 1100 })))
        .child(div().text_color(col).child(word))
        .when(!!r.context, (el) => el.child(div().text_color(ctxCol).child(fmtTok(r.context))));
    })() : null;
    const head = h_flex().pt(z(9)).pb(z(1)).px(z(6)).gap(z(6)).items_center()
      .child(h_flex().flex_1().min_w(0).gap(z(5)).items_center()
        .when(!!r.pinned, (el) => el.child(div().id(`pin-${r.name}`).text_size(z(12)).text_color(t.accent).hover((s) => s.opacity(0.6))
          .on_click((_e, cx) => { cx.stop_propagation(); this.togglePin(r, cx); }).child("📌")))
        .child(div().truncate().text_size(z(16)).font_weight(650).text_color(t.foreground).child(this.titleOf(r))))
      .when(!look.cover, (el) => el.child(pill(false)))
      .when(!!space, (el) => el.child(div().h(z(20)).px(z(7)).rounded(z(10)).flex().items_center().text_size(z(11)).bg(t.selected).text_color(t.muted).flex_none().child(space.active ? "● space" : "space")))
      .when(!!agentEl, (el) => el.child(agentEl));

    const extra = [];
    if (look.worktrees && r.worktrees?.length) extra.push(`${r.worktrees.length} worktree${r.worktrees.length > 1 ? "s" : ""}`);
    if (!r.has_remote) extra.push("no remote");
    const branch = look.branch ? div().px(z(6)).pb(z(7)).text_size(z(12.5)).text_color(t.muted).truncate()
      .child([r.branch || "(no branch)", ...extra].join("  ·  ")) : null;
    const note = look.note && rc.note ? div().px(z(6)).pb(z(7)).text_size(z(12.5)).text_color(t.foreground).truncate().child(rc.note) : null;

    let chipsEl = null;
    if (look.chips) {
      const chips = [];
      if (r.ahead) chips.push(["bad", `↑${r.ahead} unpushed`]);
      else if (r.has_remote && !r.has_upstream && r.last_commit) chips.push(["bad", "not pushed"]);
      if (r.conflicts) chips.push(["bad", `${r.conflicts} conflict${r.conflicts > 1 ? "s" : ""}`]);
      if (r.behind) chips.push(["warn", `↓${r.behind} behind`]);
      if (r.staged) chips.push(["warn", `${r.staged} staged`]);
      if (r.modified) chips.push(["warn", `${r.modified} modified`]);
      if (r.untracked) chips.push(["warn", `${r.untracked} untracked`]);
      if (r.stashes) chips.push(["info", `${r.stashes} stash${r.stashes > 1 ? "es" : ""}`]);
      if (!chips.length) chips.push(["ok", "in sync"]);
      const chipCol = (c) => (c === "info" ? t.accent : sc(c));
      chipsEl = h_flex().px(z(6)).pb(z(8)).gap(z(5)).flex_wrap()
        .children(chips.map(([c, text]) => div().h(z(19)).px(z(8)).rounded(z(10)).border(1).border_color(chipCol(c)).flex().items_center().text_size(z(11.5)).text_color(chipCol(c)).child(text)));
    }

    const line = (when, whenCol, text) => h_flex().px(z(6)).pb(z(3)).gap(z(6)).text_size(z(12.5)).min_w(0)
      .child(div().flex_none().text_color(whenCol).child(when)).child(div().flex_1().min_w(0).truncate().text_color(t.foreground).child(text));
    const acts = [];
    if (look.activity) {
      if (r.edited && r.edited >= r.last_commit) acts.push(line(`edited ${ago(r.edited)}`, sc("warn"), "uncommitted work"));
      acts.push(line(r.last_commit ? `committed ${ago(r.last_commit)}` : "", t.muted, r.last_subject || "no commits"));
    }

    const iss = this.issues[r.name];
    const issueEl = look.issues && iss?.next ? h_flex().id(`iss-${r.name}`).px(z(6)).pt(z(3)).gap(z(6)).text_size(z(12.5)).min_w(0)
      .hover((s) => s.text_color(t.accent)).on_click((_e, cx) => { cx.stop_propagation(); openUrl(iss.next.url); })
      .child(div().flex_none().w(z(34)).text_size(z(11.5)).text_color(t.muted).child("Next"))
      .child(div().flex_none().text_color(t.muted).child(`#${iss.next.number}`))
      .child(div().flex_1().min_w(0).truncate().child(iss.next.title))
      .when(iss.open > 1, (el) => el.child(div().flex_none().text_size(z(11.5)).text_color(t.dim).child(`+${iss.open - 1}`))) : null;

    const week = look.week ? h_flex().px(z(6)).pt(z(5)).gap(z(6)).text_size(z(12)).text_color(t.muted)
      .child(div().flex_shrink(1).min_w(0).overflow_hidden().whitespace_nowrap().text_color(sc("ok")).child("▮".repeat(Math.min(r.commits_7d || 0, 12)) || "–"))
      .child(div().flex_none().child(`${r.commits_7d || 0} this week`)) : null;

    // Selected: the border turns accent and a soft halo shows around it. The halo's room is
    // there on every card, so selecting never changes a card's size.
    const card = v_flex().w_full().flex_1().p(z(6)).pb(z(10)).rounded(z(14)).bg(t.background).border(1).border_color(selected ? t.accent : t.border)
      .when(!selected, (el) => el.hover((s) => s.border_color(t.input)))
      .when(r.level === "stale" && !selected, (el) => el.opacity(0.78))
      .when(!!coverEl, (el) => el.child(coverEl)).child(head)
      .when(!!branch, (el) => el.child(branch)).when(!!note, (el) => el.child(note)).when(!!chipsEl, (el) => el.child(chipsEl))
      .children(acts).when(!!issueEl, (el) => el.child(issueEl)).when(!!week, (el) => el.child(week));
    return div().id(`card-${r.name}`).flex_none().w(w + 4).max_w_full().flex().flex_col().p(2).rounded(z(14) + 3).bg(selected ? t.accent + "40" : CLEAR)
      .on_click((e, cx) => { this.h.focus(); this.select(r.name); if (e.click_count >= 2) this.open(r); cx.notify(); })
      .child(card);
  }
}
