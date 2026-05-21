import { EditorState, Compartment, StateField, StateEffect, RangeSet, RangeSetBuilder } from "@codemirror/state";
import { EditorView, keymap, lineNumbers, highlightActiveLine, gutter, GutterMarker } from "@codemirror/view";
import { markdown } from "@codemirror/lang-markdown";
import { get } from "svelte/store";
import { comments } from "./comments-store";
import type { Comment } from "./types";

export type EditorOptions = {
  value: string;
  onChange: (next: string) => void;
};

let activeView: EditorView | null = null;

type MarkerClickCallback = (line: number) => void;
type EmptyGutterClickCallback = (line: number, text: string) => void;

let onMarkerClickCb: MarkerClickCallback | null = null;
let onEmptyGutterClickCb: EmptyGutterClickCallback | null = null;

export function setOnMarkerClick(cb: MarkerClickCallback | null): void {
  onMarkerClickCb = cb;
}

export function setOnEmptyGutterClick(cb: EmptyGutterClickCallback | null): void {
  onEmptyGutterClickCb = cb;
}

type CommentMarkerKind = "open" | "relocated";

class CommentMarker extends GutterMarker {
  constructor(readonly kind: CommentMarkerKind, readonly count: number) {
    super();
  }
  override eq(other: GutterMarker): boolean {
    return other instanceof CommentMarker && other.kind === this.kind && other.count === this.count;
  }
  override toDOM(): HTMLElement {
    const el = document.createElement("div");
    el.className = `cm-comment-marker cm-comment-marker-${this.kind}`;
    el.title =
      this.count === 1
        ? "1 comment on this line"
        : `${this.count} comments on this line`;
    return el;
  }
}

// Effect dispatched whenever the comments store updates, so the gutter recomputes.
const refreshCommentMarkers = StateEffect.define<Comment[]>();

const commentMarkerField = StateField.define<RangeSet<GutterMarker>>({
  create(state) {
    return buildMarkerSet(state, get(comments));
  },
  update(set, tr) {
    let next = set.map(tr.changes);
    for (const e of tr.effects) {
      if (e.is(refreshCommentMarkers)) {
        next = buildMarkerSet(tr.state, e.value);
      }
    }
    return next;
  },
});

function buildMarkerSet(state: EditorState, list: Comment[]): RangeSet<GutterMarker> {
  // Group by line, choosing strongest status (relocated > open). Ignore resolved/orphaned.
  const byLine = new Map<number, { kind: CommentMarkerKind; count: number }>();
  for (const c of list) {
    if (c.status !== "open" && c.status !== "relocated") continue;
    const lineNum = c.line_start;
    if (lineNum < 1 || lineNum > state.doc.lines) continue;
    const existing = byLine.get(lineNum);
    if (!existing) {
      byLine.set(lineNum, { kind: c.status, count: 1 });
    } else {
      existing.count += 1;
      if (c.status === "relocated") existing.kind = "relocated";
    }
  }

  const sortedLines = [...byLine.keys()].sort((a, b) => a - b);
  const builder = new RangeSetBuilder<GutterMarker>();
  for (const lineNum of sortedLines) {
    const info = byLine.get(lineNum)!;
    const line = state.doc.line(lineNum);
    builder.add(line.from, line.from, new CommentMarker(info.kind, info.count));
  }
  return builder.finish();
}

const commentGutter = gutter({
  class: "cm-comments-gutter",
  markers: (view) => view.state.field(commentMarkerField),
  initialSpacer: () => new CommentMarker("open", 1),
  domEventHandlers: {
    mousedown(view, line) {
      const lineNum = view.state.doc.lineAt(line.from).number;
      const set = view.state.field(commentMarkerField);
      let hasMarker = false;
      set.between(line.from, line.from, () => {
        hasMarker = true;
      });
      if (hasMarker) {
        if (onMarkerClickCb) onMarkerClickCb(lineNum);
      } else {
        if (onEmptyGutterClickCb) {
          const text = view.state.doc.line(lineNum).text;
          onEmptyGutterClickCb(lineNum, text);
        }
      }
      return true;
    },
  },
});

export function codeMirror(node: HTMLElement, opts: EditorOptions) {
  let current = opts;
  const themeCompartment = new Compartment();

  const state = EditorState.create({
    doc: opts.value,
    extensions: [
      lineNumbers(),
      commentMarkerField,
      commentGutter,
      highlightActiveLine(),
      keymap.of([]),
      markdown(),
      EditorView.lineWrapping,
      themeCompartment.of(theme()),
      EditorView.updateListener.of((update) => {
        if (update.docChanged) {
          current.onChange(update.state.doc.toString());
        }
      }),
    ],
  });

  const view = new EditorView({
    state,
    parent: node,
  });
  activeView = view;

  // Subscribe to comments store; re-dispatch a refresh effect whenever it changes.
  const unsubscribe = comments.subscribe((list) => {
    // The subscribe call fires synchronously with the initial value; the field
    // already builds from `get(comments)` in its `create()`, so this is mostly
    // redundant on first tick but still harmless and keeps subsequent updates
    // in sync.
    view.dispatch({ effects: refreshCommentMarkers.of(list) });
  });

  return {
    update(next: EditorOptions) {
      current = next;
      const editorText = view.state.doc.toString();
      if (next.value !== editorText) {
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: next.value },
        });
      }
    },
    destroy() {
      unsubscribe();
      if (activeView === view) activeView = null;
      view.destroy();
    },
  };
}

export function getCurrentLine(): { number: number; text: string } | null {
  if (!activeView) return null;
  const sel = activeView.state.selection.main;
  const line = activeView.state.doc.lineAt(sel.head);
  return { number: line.number, text: line.text };
}

export function focusLine(lineNumber: number): void {
  if (!activeView) return;
  if (lineNumber < 1) return;
  const total = activeView.state.doc.lines;
  const target = Math.min(lineNumber, total);
  const line = activeView.state.doc.line(target);
  activeView.dispatch({
    selection: { anchor: line.from, head: line.from },
    scrollIntoView: true,
  });
  activeView.focus();
}

function theme() {
  return EditorView.theme({
    "&": {
      height: "100%",
      fontSize: "14px",
    },
    ".cm-scroller": {
      fontFamily:
        '"JetBrains Mono", "SFMono-Regular", Consolas, "Liberation Mono", monospace',
      lineHeight: "1.55",
    },
    ".cm-content": {
      padding: "1rem 0",
    },
    ".cm-gutters": {
      background: "transparent",
      borderRight: "1px solid #e8e2d2",
      color: "#9c9685",
    },
    ".cm-comments-gutter": {
      width: "16px",
      cursor: "pointer",
    },
    ".cm-comments-gutter .cm-gutterElement": {
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
    },
    ".cm-comment-marker": {
      width: "8px",
      height: "8px",
      borderRadius: "50%",
      transition: "transform 120ms ease",
      boxShadow: "0 0 0 1px rgba(255,255,255,0.6)",
    },
    ".cm-comment-marker:hover": {
      transform: "scale(1.35)",
    },
    ".cm-comment-marker-open": {
      background: "#0f62fe",
    },
    ".cm-comment-marker-relocated": {
      background: "#f1c21b",
    },
  });
}
