// Self-hosted fonts. All four FOSS families ship via @fontsource (no calls
// to Google Fonts — GDPR-friendly). Verdana is system-only on purpose: it's
// a proprietary Microsoft font we can't bundle.

import "@fontsource/ibm-plex-sans/400.css";
import "@fontsource/ibm-plex-sans/700.css";
import "@fontsource/ibm-plex-sans/400-italic.css";
import "@fontsource/ibm-plex-serif/400.css";
import "@fontsource/ibm-plex-serif/700.css";
import "@fontsource/ibm-plex-serif/400-italic.css";
import "@fontsource/eb-garamond/400.css";
import "@fontsource/eb-garamond/700.css";
import "@fontsource/eb-garamond/400-italic.css";
import "@fontsource/lato/400.css";
import "@fontsource/lato/700.css";
import "@fontsource/lato/400-italic.css";

export type FontKey = "ibm-plex-sans" | "ibm-plex-serif" | "eb-garamond" | "lato" | "verdana";

export type FontOption = {
  key: FontKey;
  label: string;
  /** css font-family stack — exact value pushed to the --steelpage-font variable */
  stack: string;
  /** when true we don't ship the font ourselves and rely on the user's system */
  systemOnly?: boolean;
};

export const FONTS: FontOption[] = [
  {
    key: "ibm-plex-sans",
    label: "IBM Plex Sans (default)",
    stack: `"IBM Plex Sans", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`,
  },
  {
    key: "ibm-plex-serif",
    label: "IBM Plex Serif",
    stack: `"IBM Plex Serif", Cambria, Georgia, serif`,
  },
  {
    key: "eb-garamond",
    label: "Garamond (EB Garamond)",
    stack: `"EB Garamond", Garamond, "Times New Roman", serif`,
  },
  {
    key: "lato",
    label: "Lato",
    stack: `"Lato", "Helvetica Neue", Helvetica, Arial, sans-serif`,
  },
  {
    key: "verdana",
    label: "Verdana (system)",
    stack: `Verdana, Geneva, sans-serif`,
    systemOnly: true,
  },
];

export const DEFAULT_FONT: FontKey = "ibm-plex-sans";

export function stackFor(key: string | null | undefined): string {
  const f = FONTS.find((x) => x.key === key);
  return (f ?? FONTS[0]).stack;
}

// applyFont pushes the chosen stack onto a CSS variable everyone in
// styles/app.css consults. Called from a reactive subscriber on `me`.
export function applyFont(key: string | null | undefined): void {
  if (typeof document === "undefined") return;
  document.documentElement.style.setProperty("--steelpage-font", stackFor(key));
}
