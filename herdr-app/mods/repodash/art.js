// Cover art, ported from RepoDash's web page (internal/server/web/index.html, "cover art"):
// same hash, same random sequence, so a repo gets the same art in both. The page draws
// aurora/deep as SVG and gradient as CSS; here all three become SVG files in ./.covers
// (image() only reads files), and "name" is drawn natively by the card.

import { appHome } from "herdr";
import * as fs from "fs/promises";

export function hash(str) { let h = 2166136261; for (const c of str) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619); } return h >>> 0; }
export function rng(seed) {
  return () => {
    seed = seed + 0x6D2B79F5 | 0; let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
    t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t; return ((t ^ t >>> 14) >>> 0) / 4294967296;
  };
}

export const PAL = [["#EEE6D8", "#D9573B", "#2F5D62", "#E8B04A", "#243035"], ["#E6EDF3", "#274C77", "#6096BA", "#E3A812", "#1B2A3A"], ["#EFEAE0", "#7A9E7E", "#C7603B", "#3B4B59", "#E2C28F"],
  ["#ECE9F5", "#5B4B8A", "#F2A65A", "#2A9D8F", "#2B2B3A"], ["#F3E7E2", "#B5446E", "#2D6A6A", "#F0B67F", "#2E2A33"], ["#E7EFE4", "#2E6F40", "#E9A23B", "#C84B31", "#1F2D26"],
  ["#F1ECDB", "#1D4E89", "#E4572E", "#F2C14E", "#222222"], ["#E4EBEE", "#0B6E6E", "#F26B5B", "#F6D55C", "#173F4F"], ["#F0E4EA", "#3C3F8F", "#E26D5A", "#7BC8A4", "#25243A"], ["#E8EDE6", "#C2452D", "#3F7F6E", "#EAB308", "#2A2F2B"]];

export function nameCover(name) {
  const r = rng(hash(name)), P = PAL[hash(name) % PAL.length], bg = [P[4], P[1], P[2], P[0]][Math.floor(r() * 4)];
  return { bg, ink: bg === P[0] || bg === P[3] ? P[4] : "#ffffff" };
}

export const LANG_COLOR = { Go: "#2A9CB8", Python: "#3E6FA3", TypeScript: "#3178C6", JavaScript: "#B59B0C", Rust: "#B5532A", Swift: "#E0533D", Shell: "#4E9A3C", "C++": "#C2417A", C: "#5C6B7A", Ruby: "#B8322A", Java: "#B0722A", Kotlin: "#7F52FF", "C#": "#2F8F3A", PHP: "#6C78AF", Lua: "#2C2D72", Dart: "#0B7FA8", Zig: "#C27A1A", Elixir: "#6E4A7E", Haskell: "#5E5086", Scala: "#C22D40", Vue: "#3B9E6E", Svelte: "#D9481C", QML: "#41A15C" };
export function mixHex(a, b, t) {
  const p = (h) => [1, 3, 5].map((i) => parseInt(h.slice(i, i + 2), 16)), x = p(a), y = p(b);
  return "#" + x.map((v, i) => Math.round(v + (y[i] - v) * t).toString(16).padStart(2, "0")).join("");
}

// hsl() as hex: the SVG renderer is surest with plain colours.
function hsl(h, s, l) {
  h = ((h % 360) + 360) % 360; s /= 100; l /= 100;
  const k = (n) => (n + h / 30) % 12, a = s * Math.min(l, 1 - l);
  const f = (n) => Math.round(255 * (l - a * Math.max(-1, Math.min(k(n) - 3, Math.min(9 - k(n), 1))))).toString(16).padStart(2, "0");
  return `#${f(0)}${f(8)}${f(4)}`;
}

const HEAD = '<svg xmlns="http://www.w3.org/2000/svg" width="720" height="240" viewBox="0 0 360 120">';

function aurora(name) {
  const r = rng(hash(name)), h0 = Math.floor(r() * 360);
  const blobs = [0, 40, -45, 95].map((o) => {
    const cx = Math.floor(r() * 360), cy = Math.floor(r() * 120), rx = 60 + Math.floor(r() * 70), ry = 22 + Math.floor(r() * 30);
    const fill = hsl(h0 + o, 95, 52 + Math.floor(r() * 10)), rot = Math.floor(r() * 50 - 25);
    return `<ellipse cx="${cx}" cy="${cy}" rx="${rx}" ry="${ry}" fill="${fill}" transform="rotate(${rot} 180 60)"/>`;
  }).join("");
  return `${HEAD}<defs><filter id="b" x="-30%" y="-80%" width="160%" height="260%"><feGaussianBlur stdDeviation="17"/></filter></defs>`
    + `<rect width="360" height="120" fill="${hsl(h0 + 200, 50, 7)}"/><g filter="url(#b)" style="mix-blend-mode:screen" opacity=".92">${blobs}</g></svg>`;
}

function deep(name) {
  const r = rng(hash(name)), h0 = Math.floor(r() * 360), sp = 40 + Math.floor(r() * 40);
  const c = (o, l) => hsl(h0 + o, 62 + Math.floor(r() * 16), l);
  const rot = 20 + Math.floor(r() * 50), s0 = c(0, 16), s1 = c(sp, 24), s2 = c(sp * 2, 30);
  const rcx = (0.2 + r() * 0.6).toFixed(3), rcy = (0.1 + r() * 0.8).toFixed(3), glow = c(sp + 30, 52), glow2 = c(sp + 30, 52);
  return `${HEAD}<defs>`
    + `<linearGradient id="g" gradientTransform="rotate(${rot} .5 .5)"><stop offset="0" stop-color="${s0}"/><stop offset=".55" stop-color="${s1}"/><stop offset="1" stop-color="${s2}"/></linearGradient>`
    + `<radialGradient id="r" cx="${rcx}" cy="${rcy}" r=".6"><stop offset="0" stop-color="${glow}" stop-opacity=".6"/><stop offset="1" stop-color="${glow2}" stop-opacity="0"/></radialGradient>`
    + `<filter id="n"><feTurbulence type="fractalNoise" baseFrequency=".85" numOctaves="2" stitchTiles="stitch"/><feColorMatrix values="0 0 0 0 1 0 0 0 0 1 0 0 0 0 1 0 0 0 .5 0"/></filter></defs>`
    + `<rect width="360" height="120" fill="url(#g)"/><rect width="360" height="120" fill="url(#r)"/><rect width="360" height="120" filter="url(#n)" opacity=".1"/></svg>`;
}

// The page's CSS mesh: pastel radial blobs over a two-colour wash.
function gradient(name) {
  const rnd = rng(hash(name)), h0 = Math.floor(rnd() * 360);
  const pastel = (h) => hsl(h, 72 + Math.floor(rnd() * 18), 80 + Math.floor(rnd() * 9));
  const spread = [0, 28 + rnd() * 30, -(28 + rnd() * 30), 90 + rnd() * 70];
  const blobs = spread.map((off) => {
    const rx = 55 + Math.floor(rnd() * 35), ry = 80 + Math.floor(rnd() * 50), x = Math.floor(rnd() * 100), y = Math.floor(rnd() * 100);
    return { rx: rx * 3.6, ry: ry * 1.2, x: x * 3.6, y: y * 1.2, color: pastel(h0 + off) };
  });
  const angle = Math.floor(rnd() * 360), c1 = pastel(h0 - 20), c2 = pastel(h0 + 50);
  const defs = blobs.map((b, i) => `<radialGradient id="b${i}"><stop offset="0" stop-color="${b.color}"/><stop offset=".72" stop-color="${b.color}" stop-opacity="0"/></radialGradient>`).join("");
  // CSS paints its first layer on top: draw the last blob first.
  const shapes = blobs.map((b, i) => `<ellipse cx="${b.x}" cy="${b.y}" rx="${b.rx}" ry="${b.ry}" fill="url(#b${i})"/>`).reverse().join("");
  return `${HEAD}<defs><linearGradient id="w" gradientTransform="rotate(${angle - 90} .5 .5)"><stop offset="0" stop-color="${c1}"/><stop offset="1" stop-color="${c2}"/></linearGradient>${defs}</defs>`
    + `<rect width="360" height="120" fill="url(#w)"/>${shapes}</svg>`;
}

const MAKERS = { aurora, deep, gradient };
export const STYLES = ["aurora", "deep", "gradient", "name"];

// name+style -> "ready" | "pending". Written once per app run; the files are tiny.
const files = new Map();

/** The art file for image() (a path under the app folder), or null while it is written. */
export function artPath(name, style, cx) {
  const make = MAKERS[style];
  if (!make) return null;
  const key = `${style}-${hash(name).toString(36)}`, rel = `mods/repodash/.covers/art-${key}.svg`;
  const state = files.get(key);
  if (state === "ready") return rel;
  if (!state && cx) {
    files.set(key, "pending");
    cx.spawn(async (cx) => {
      try {
        await fs.mkdir(`${appHome()}/mods/repodash/.covers`, { recursive: true }).catch(() => {});
        await fs.writeFile(`${appHome()}/${rel}`, make(name));
        files.set(key, "ready");
      } catch { files.delete(key); }
      cx.notify();
    });
  }
  return null;
}
