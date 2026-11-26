import React from "react";
import { getSourceTheme } from "../utils/formatters";

interface SourceIconProps {
  source: string;
  className?: string;
}

const DARK_SOURCE_BADGE_CLASSES: Record<string, string> = {
  "Claude Code": "dark:bg-[#2B1914] dark:text-[#FDBA9E] dark:border-[#6B3322]",
  "Gemini CLI": "dark:bg-[#142242] dark:text-[#93C5FD] dark:border-[#1E3A8A]",
  Codex: "dark:bg-[#072B21] dark:text-[#6EE7B7] dark:border-[#065F46]",
  Antigravity: "dark:bg-[#1E193B] dark:text-[#C4B5FD] dark:border-[#4C1D95]",
  "DeepSeek Harness": "dark:bg-[#172047] dark:text-[#A5B4FC] dark:border-[#3730A3]",
  "Pi Agent": "dark:bg-[#0B2D2A] dark:text-[#5EEAD4] dark:border-[#115E59]",
};

export const SourceIcon = React.memo(function SourceIcon({
  source,
  className = "w-4 h-4",
}: SourceIconProps) {
  const theme = getSourceTheme(source);

  if (source === "Claude Code") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" className="fill-[#FDF4F0] dark:fill-[#331E17]" />
        <path
          d="M10 4.2V15.8M4.2 10H15.8M5.9 5.9L14.1 14.1M14.1 5.9L5.9 14.1"
          stroke="#D97757"
          strokeWidth="1.75"
          strokeLinecap="round"
        />
      </svg>
    );
  }

  if (source === "Gemini CLI") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" className="fill-[#EFF6FF] dark:fill-[#172554]" />
        <path
          d="M10 3.8C10.4 7.4 12.6 9.6 16.2 10C12.6 10.4 10.4 12.6 10 16.2C9.6 12.6 7.4 10.4 3.8 10C7.4 9.6 9.6 7.4 10 3.8Z"
          className="fill-[#2563EB] dark:fill-[#60A5FA]"
        />
      </svg>
    );
  }

  if (source === "Codex") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" className="fill-[#ECFDF5] dark:fill-[#063A2C]" />
        <path
          d="M7.5 6.8L4.5 10L7.5 13.2M12.5 6.8L15.5 10L12.5 13.2M10.8 5.8L9.2 14.2"
          className="stroke-[#059669] dark:stroke-[#34D399]"
          strokeWidth="1.7"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    );
  }

  if (source === "Antigravity") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" className="fill-[#F5F3FF] dark:fill-[#221A44]" />
        <path
          d="M10 4.5L15 14.5H12.4L10 9.4L7.6 14.5H5L10 4.5Z"
          className="fill-[#7C3AED] dark:fill-[#A78BFA]"
        />
        <circle cx="10" cy="14.2" r="1.4" className="fill-[#7C3AED] dark:fill-[#A78BFA]" />
      </svg>
    );
  }

  if (source === "DeepSeek Harness") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" className="fill-[#EEF2FF] dark:fill-[#1A204C]" />
        <path
          d="M4.2 12.8c1.5-4.1 3-4.1 4.5 0s3 4.1 4.5 0 2.9-3.6 2.6-1.1"
          className="stroke-[#4D6BFE] dark:stroke-[#818CF8]"
          strokeWidth="1.7"
          strokeLinecap="round"
        />
        <circle cx="14.6" cy="6.1" r="1.2" className="fill-[#4D6BFE] dark:fill-[#818CF8]" />
      </svg>
    );
  }

  if (source === "Pi Agent") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" className="fill-[#F0FDFA] dark:fill-[#0F3330]" />
        <path
          d="M5.4 7.1h9.2M8 7.1v6.8M12 7.1v6.8"
          className="stroke-[#0F766E] dark:stroke-[#2DD4BF]"
          strokeWidth="1.8"
          strokeLinecap="round"
        />
      </svg>
    );
  }

  return (
    <span
      className={`inline-flex items-center justify-center rounded text-[9px] font-bold tracking-wider ${theme.bgTint} ${theme.textClass} dark:bg-slate-800 dark:text-slate-200 ${className}`}
    >
      {theme.shortCode}
    </span>
  );
});

interface SourceBadgeProps {
  source: string;
  compact?: boolean;
}

export const SourceBadge = React.memo(function SourceBadge({
  source,
  compact = false,
}: SourceBadgeProps) {
  const theme = getSourceTheme(source);
  const darkClasses =
    DARK_SOURCE_BADGE_CLASSES[source] ??
    "dark:bg-slate-800/80 dark:text-slate-200 dark:border-slate-700";

  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium ${theme.bgTint} ${theme.textClass} ${theme.borderClass} ${darkClasses}`}
    >
      <SourceIcon source={source} className="h-3.5 w-3.5 shrink-0" />
      {!compact && <span className="truncate">{source}</span>}
    </span>
  );
});
