// Mention segmentation for rendering comment bodies. Mirrors the backend
// matcher (internal/notifications/mentions.go): '@' opens a mention only at a
// word boundary, candidates match longest-first, and a match must end at a
// word boundary too.

export type Segment = { text: string; mention: boolean };

function isBoundary(ch: string): boolean {
  return /[\s\p{P}\p{S}]/u.test(ch);
}

export function splitMentions(body: string, names: string[]): Segment[] {
  if (!body) return [];
  const candidates = names
    .filter((n) => n.trim() !== "")
    .sort((a, b) => b.length - a.length);
  if (candidates.length === 0) return [{ text: body, mention: false }];

  const segments: Segment[] = [];
  let plainStart = 0;
  let i = 0;
  while (i < body.length) {
    if (body[i] !== "@" || (i > 0 && !isBoundary(body[i - 1]))) {
      i++;
      continue;
    }
    const rest = body.slice(i + 1);
    const name = candidates.find((n) => {
      if (!rest.startsWith(n)) return false;
      const after = rest[n.length];
      return after === undefined || isBoundary(after);
    });
    if (!name) {
      i++;
      continue;
    }
    if (plainStart < i) {
      segments.push({ text: body.slice(plainStart, i), mention: false });
    }
    segments.push({ text: `@${name}`, mention: true });
    i += name.length + 1;
    plainStart = i;
  }
  if (plainStart < body.length) {
    segments.push({ text: body.slice(plainStart), mention: false });
  }
  return segments;
}
