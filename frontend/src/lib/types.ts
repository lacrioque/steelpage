export type SteelpageDocument = {
  path: string;
  title: string;
  frontmatter: Record<string, unknown>;
  markdown: string;
  html: string;
  sha: string;
  updated: string;
  comments: unknown[];
  viewing_ref?: string;
};

export type HistoryEntry = {
  sha: string;
  author_name: string;
  author_email: string;
  date: string;
  message: string;
};

export type TreeEntry = {
  path: string;
  is_dir: boolean;
};

export type ApiError = Error & { status?: number };

export type CommentAuthor = {
  id: number;
  display_name: string;
};

export type CommentStatus = "open" | "resolved" | "orphaned" | "relocated";

export type SearchResult = {
  path: string;
  title: string;
  heading_snippet: string;
  body_snippet: string;
  rank: number;
};

export type Comment = {
  id: number;
  path: string;
  line_start: number;
  line_end: number;
  anchor_text: string;
  document_sha: string;
  author: CommentAuthor;
  body: string;
  status: CommentStatus;
  created_at: string;
  updated_at: string;
  resolved_at: string | null;
  reply_to: number | null;
};
