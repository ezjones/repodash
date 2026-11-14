// The inspector: the selected repo's details beside the cards, like the web page's.
// Status and why, actions (open, lazygit, web, copy cd), agent and space, recent commits
// (click one for author, body and files), open issues, worktrees, notes.

import { div } from "gpui-kit";
import { v_flex, h_flex, TextView } from "gpui-base";
import { ago, fmtTok } from "./util.js";

const SECTION = (t, label, extra) => h_flex().pt(4).gap(6).items_baseline()
  .child(div().text_size(11).font_weight(600).text_color(t.dim).child(label.toUpperCase()))
  .when(!!extra, (el) => el.child(div().text_size(11).text_color(t.dim).child(extra)));

function button(t, id, label, hint, onClick) {
  return h_flex().id(id).h(28).px(10).gap(6).rounded(7).items_center().flex_none().text_size(12.5)
    .border(1).border_color(t.border).text_color(t.foreground).hover((s) => s.bg(t.hover))
    .on_click((e, cx) => { cx.stop_propagation(); onClick(e, cx); })
    .child(label).when(!!hint, (el) => el.child(div().text_size(11).text_color(t.dim).child(hint)));
}

// Lines J/K move over and o / Enter opens: each gets the next index, and a highlight when it
// has the keyboard. Collected afresh on every render.
function rowsOf(v, t) {
  const rows = [];
  return {
    rows,
    add(el, id, act) {
      const i = rows.length;
      rows.push({ id, act });
      return el.when(v.ifocus === i, (e) => e.bg(t.selected).border_l(2).border_color(t.accent));
    },
  };
}

const RANGES = [["today", "Today", "d"], ["yesterday", "Yesterday", "D"], ["week", "Week", "w"], ["month", "Month", "m"]];

function tabs(v, t) {
  const tab = (id, label, key, on) => div().id(`tab-${id}`).h(26).px(12).rounded(13).flex().items_center().text_size(12.5)
    .bg(on ? t.selected : "#00000000").text_color(on ? t.foreground : t.muted).hover((s) => s.bg(t.hover))
    .on_click((_e, cx) => { v.setInspMode(id); cx.notify(); })
    .child(label).child(div().pl(6).text_size(11).text_color(t.dim).child(key));
  return h_flex().flex_none().px(12).pt(10).pb(6).gap(4).border_b(1).border_color(t.border)
    .child(tab("repo", "This repo", "1", v.inspMode !== "all")).child(tab("all", "All", "2", v.inspMode === "all"));
}

export function renderInspector(v, r, cx, t, sc, ctx) {
  const root = v_flex().id("insp").w(v.inspW).flex_none().h_full().min_h(0).border_l(1).border_color(t.border).bg(t.background).child(tabs(v, t));
  if (v.inspMode === "all") return root.child(renderAll(v, cx, t, sc));
  if (!r) { v.inspRows = []; return root.child(div().p(16).text_color(t.muted).child("Select a repo.")); }
  const R = rowsOf(v, t);
  v.ensureDetail(r, cx);
  const det = v.det?.name === r.name ? v.det : null;
  const space = v.spaceFor(r, ctx), agent = v.agentFor(r, ctx);
  const color = sc(r.level), home = r.path.replace(/^\/home\/[^/]+/, "~");

  const head = v_flex().gap(4)
    .child(h_flex().gap(8).items_center()
      .child(div().flex_1().min_w(0).truncate().text_size(18).font_weight(700).text_color(t.foreground).child(v.titleOf(r)))
      .child(h_flex().h(22).px(9).gap(6).rounded(11).items_center().text_size(12).font_weight(600).flex_none()
        .bg(r.level === "bad" ? color : t.surface).text_color(r.level === "bad" ? "#ffffff" : color)
        .child(div().size(7).rounded(4).bg(r.level === "bad" ? "#ffffff" : color)).child(v.statusLabel(r.level))))
    .child(div().text_size(12.5).text_color(t.muted).truncate().child(`${r.branch || "(no branch)"}${r.base && r.base !== r.branch ? ` from ${r.base}` : ""}  ·  ${home}`))
    .when(!!(r.reasons?.length), (el) => el.child(v_flex().pt(2).gap(1).children(r.reasons.map((x) => div().text_size(12).text_color(color).child(`· ${x}`)))));

  const actions = h_flex().gap(6).flex_wrap()
    .child(button(t, "a-open", space ? "Go to space" : "New space", "⏎", () => v.open(r)))
    .child(button(t, "a-git", "Lazygit", "L", (_e, cx) => v.openGit(r, r.path, cx)))
    .when(!!det?.web, (el) => el.child(button(t, "a-web", "GitHub ↗", null, () => v.openUrl(det.web))))
    .child(button(t, "a-copy", v.copied === r.name ? "Copied" : "Copy cd", null, (_e, cx) => { cx.write_to_clipboard(`cd ${r.path}`); v.copied = r.name; cx.notify(); }))
    .child(button(t, "a-notes", "Notes", "n", () => v.openNotes(r)))
    .child(button(t, "a-pin", r.pinned ? "Unpin" : "Pin", "p", (_e, cx) => v.togglePin(r, cx)))
    .child(button(t, "a-hide", r.hidden ? "Unhide" : "Hide", "x", (_e, cx) => v.toggleHidden(r, cx)));

  const facts = v_flex().gap(3).text_size(12.5)
    .child(h_flex().gap(6).child(div().w(60).flex_none().text_color(t.dim).child("Space"))
      .child(div().text_color(space ? t.foreground : t.muted).child(space ? `${space.name}${space.active ? "  (shown)" : ""}` : "none yet")))
    .child(h_flex().gap(6).child(div().w(60).flex_none().text_color(t.dim).child("Agent"))
      .child(agent ? h_flex().gap(6).child(div().text_color(t[agent.status] ?? t.muted).child({ working: "working", blocked: "waiting for you", done: "finished", idle: "idle" }[agent.status] ?? agent.status))
        .when(!!r.context, (el) => el.child(div().text_color(t.muted).child(`${fmtTok(r.context)}${r.context_size ? ` of ${fmtTok(r.context_size)} (${Math.round(100 * r.context / r.context_size)}%)` : ""}`)))
        : div().text_color(t.muted).child("none")))
    .child(h_flex().gap(6).child(div().w(60).flex_none().text_color(t.dim).child("Activity"))
      .child(div().text_color(t.foreground).child(`${r.commits_7d || 0} commits this week${r.edited && r.edited >= r.last_commit ? `, edited ${ago(r.edited)}` : ""}`)));

  // Commits
  const commits = v_flex().gap(1).child(SECTION(t, "Recent commits"));
  if (!det) commits.child(div().text_size(12.5).text_color(t.dim).child("Loading…"));
  else if (!det.commits.length) commits.child(div().text_size(12.5).text_color(t.dim).child("No commits."));
  for (const c of det?.commits ?? []) {
    const open = v.openCommit === c.hash, d = v.commitDet[c.hash];
    commits.child(R.add(v_flex().id(`c-${c.hash}`).px(6).py(4).rounded(6).gap(3).hover((s) => s.bg(t.hover)), `c-${c.hash}`, (cx) => v.toggleCommit(r, c.hash, cx))
      .when(open, (el) => el.bg(t.surface))
      .on_click((_e, cx) => { v.toggleCommit(r, c.hash, cx); })
      .child(h_flex().gap(8).text_size(12.5).min_w(0)
        .child(div().flex_none().font_family("Geist Mono").text_color(t.accent).child(c.hash.slice(0, 7)))
        .child(div().flex_1().min_w(0).truncate().text_color(t.foreground).child(c.subject))
        .child(div().flex_none().text_size(11.5).text_color(t.dim).child(ago(c.at))))
      .when(open, (el) => el.child(!d ? div().text_size(12).text_color(t.dim).child("Loading…")
        : v_flex().gap(3).text_size(12)
          .child(div().text_color(t.muted).child(`${d.author} · ${new Date(d.at * 1000).toLocaleString()}`))
          .when(!!d.body, (el) => el.child(div().text_color(t.foreground).child(d.body)))
          .when(!!(d.files?.length), (el) => el.child(v_flex().font_family("Geist Mono").text_size(11.5).text_color(t.muted).children(d.files.slice(0, 20).map((f) => div().truncate().child(f)))))
          .when(!!det.web, (el) => el.child(div().id(`cw-${c.hash}`).text_color(t.accent).hover((s) => s.text_color(t.foreground))
            .on_click((_e, cx) => { cx.stop_propagation(); v.openUrl(`${det.web}/commit/${c.hash}`); }).child("View on GitHub ↗"))))));
  }

  // Issues
  const iss = v.issues[r.name];
  const issues = iss?.open_list?.length ? v_flex().gap(1).child(SECTION(t, "Issues", `${iss.open} open`))
    .children(iss.open_list.slice(0, 8).map((i) => R.add(h_flex().id(`i-${i.number}`).px(6).py(3).rounded(6).gap(8).text_size(12.5).min_w(0), `i-${i.number}`, () => v.openUrl(i.url))
      .hover((s) => s.bg(t.hover)).on_click(() => v.openUrl(i.url))
      .child(div().flex_none().text_color(t.muted).child(`#${i.number}`))
      .child(div().flex_1().min_w(0).truncate().text_color(t.foreground).child(i.title)))) : null;

  // Worktrees
  const wts = r.worktrees?.length ? v_flex().gap(1).child(SECTION(t, "Worktrees", String(r.worktrees.length)))
    .children(r.worktrees.map((w, i) => R.add(h_flex().id(`w-${i}`).px(6).py(3).rounded(6).gap(8).text_size(12.5).min_w(0).items_center(), `w-${i}`, () => v.openFolder(w.path))
      .hover((s) => s.bg(t.hover)).on_click(() => v.openFolder(w.path))
      .child(div().flex_1().min_w(0).truncate().text_color(t.foreground).child(w.branch || w.path))
      .when(!!w.merged, (el) => el.child(div().text_size(11).text_color(sc("ok")).child("merged")))
      .when(!!w.dirty, (el) => el.child(div().text_size(11).text_color(sc("warn")).child(`${w.dirty} dirty`)))
      .child(div().id(`wg-${i}`).flex_none().px(6).rounded(5).text_size(11).text_color(t.dim).hover((s) => s.bg(t.raised).text_color(t.foreground))
        .on_click((_e, cx) => { cx.stop_propagation(); v.openGit(r, w.path, cx); }).child("lazygit")))) : null;

  // Notes
  const notes = v_flex().gap(4).child(SECTION(t, "Notes"))
    .child(!det ? div().text_size(12.5).text_color(t.dim).child("Loading…")
      : det.notes ? div().w_full().text_size(12.5).text_color(t.foreground).child(TextView.markdown(`notes-${r.name}`, det.notes))
        : div().text_size(12.5).text_color(t.dim).child("No notes yet. n writes some."));
  v.inspRows = R.rows;
  if (v.ifocus >= R.rows.length) v.ifocus = R.rows.length - 1;

  return root.child(v_flex().id("insp-scroll").size_full().min_h(0).overflow_y_scrollbar().p(16).gap(14)
    .child(head).child(actions).child(facts).child(commits)
    .when(!!issues, (el) => el.child(issues)).when(!!wts, (el) => el.child(wts)).child(notes));
}

// The All tab: work across every repo in a range (commits on any branch, uncommitted edits),
// most recent first. A line selects its repo's card; o / Enter goes to its space.
function renderAll(v, cx, t, sc) {
  v.ensureWork(cx);
  const R = rowsOf(v, t), w = v.work?.range === v.workRange ? v.work : null, data = w?.data;
  const ranges = h_flex().gap(4).flex_wrap().children(RANGES.map(([id, label, key]) => div().id(`rg-${id}`).h(24).px(10).rounded(12).flex().items_center().text_size(12)
    .border(1).border_color(v.workRange === id ? t.accent : t.border).bg(v.workRange === id ? t.accent : "#00000000")
    .text_color(v.workRange === id ? t.accent_foreground : t.foreground).hover((s) => s.opacity(0.85))
    .on_click((_e, cx) => { v.setRange(id); cx.notify(); }).child(`${label} ${key}`)));
  const copy = button(t, "w-copy", v.copiedWork ? "Copied" : "Copy as markdown", null, (_e, cx) => { if (data?.markdown) { cx.write_to_clipboard(data.markdown); v.copiedWork = true; cx.notify(); } });
  const body = v_flex().gap(12);
  if (!data) body.child(div().text_size(12.5).text_color(t.dim).child(w ? "RepoDash didn't answer." : "Loading…"));
  else if (!(data.repos ?? []).length) body.child(div().text_size(12.5).text_color(t.dim).child("No work in this range."));
  for (const wr of data?.repos ?? []) {
    const repo = (v.repos ?? []).find((x) => x.name === wr.name);
    const go = () => { if (repo) { v.select(repo.name); v.open(repo); } };
    const head = R.add(h_flex().id(`wr-${wr.name}`).px(6).py(3).rounded(6).gap(8).items_center().hover((s) => s.bg(t.hover))
      .on_click((_e, cx) => { if (repo) { v.select(repo.name); cx.notify(); } })
      .child(div().flex_1().min_w(0).truncate().text_size(13.5).font_weight(650).text_color(t.foreground).child(wr.name))
      .child(div().text_size(11.5).text_color(t.dim).child(`${(wr.commits ?? []).length} commit${(wr.commits ?? []).length === 1 ? "" : "s"}`)), `wr-${wr.name}`, go);
    const lines = v_flex().gap(1).children((wr.commits ?? []).slice(0, 12).map((c) => h_flex().px(6).gap(8).text_size(12.5).min_w(0)
      .child(div().flex_none().font_family("Geist Mono").text_color(t.accent).child(c.hash.slice(0, 7)))
      .child(div().flex_1().min_w(0).truncate().text_color(t.foreground).child(c.subject))
      .child(div().flex_none().text_size(11.5).text_color(t.dim).child(ago(c.at)))))
      .when(!!wr.uncommitted, (el) => el.child(div().px(6).text_size(12.5).text_color(sc("warn")).child(`${wr.uncommitted} uncommitted file${wr.uncommitted === 1 ? "" : "s"}${wr.edited ? `, edited ${ago(wr.edited)}` : ""}`)));
    body.child(v_flex().gap(2).child(head).child(lines));
  }
  // Moving onto a line selects that repo's card, without leaving All.
  const f = R.rows[v.ifocus];
  if (f && f.id.startsWith("wr-")) { const name = f.id.slice(3); if (v.sel !== name && (v.repos ?? []).some((x) => x.name === name)) v.sel = name; }
  v.inspRows = R.rows;
  if (v.ifocus >= R.rows.length) v.ifocus = R.rows.length - 1;
  return v_flex().id("all-scroll").flex_1().min_h(0).overflow_y_scrollbar().p(16).gap(12)
    .child(h_flex().gap(8).items_center().flex_wrap().child(ranges).child(div().flex_1()).child(copy)).child(body);
}
