import React from "react";
import { getSourceTheme } from "../utils/formatters";

interface SourceIconProps {
  source: string;
  className?: string;
}

export const SourceIcon: React.FC<SourceIconProps> = ({ source, className = "w-4 h-4" }) => {
  const theme = getSourceTheme(source);

  if (source === "Claude Code") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" fill="#FDF4F0" />
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
        <rect width="20" height="20" rx="5" fill="#EFF6FF" />
        <path
          d="M10 3.8C10.4 7.4 12.6 9.6 16.2 10C12.6 10.4 10.4 12.6 10 16.2C9.6 12.6 7.4 10.4 3.8 10C7.4 9.6 9.6 7.4 10 3.8Z"
          fill="#2563EB"
        />
      </svg>
    );
  }

  if (source === "Codex") {
    return (
      <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
        <rect width="20" height="20" rx="5" fill="#ECFDF5" />
        <path
          d="M7.5 6.8L4.5 10L7.5 13.2M12.5 6.8L15.5 10L12.5 13.2M10.8 5.8L9.2 14.2"
          stroke="#059669"
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
        <rect width="20" height="20" rx="5" fill="#F5F3FF" />
        <path
          d="M10 4.5L15 14.5H12.4L10 9.4L7.6 14.5H5L10 4.5Z"
          fill="#7C3AED"
        />
        <circle cx="10" cy="14.2" r="1.4" fill="#7C3AED" />
      </svg>
    );
  }

  return (
    <span
      className={`inline-flex items-center justify-center rounded text-[9px] font-bold tracking-wider ${theme.bgTint} ${theme.textClass} ${className}`}
    >
      {theme.shortCode}
    </span>
  );
};

interface SourceBadgeProps {
  source: string;
  compact?: boolean;
}

export const SourceBadge: React.FC<SourceBadgeProps> = ({ source, compact = false }) => {
  const theme = getSourceTheme(source);
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium ${theme.bgTint} ${theme.textClass} ${theme.borderClass}`}
    >
      <SourceIcon source={source} className="h-3.5 w-3.5 shrink-0" />
      {!compact && <span className="truncate">{source}</span>}
    </span>
  );
};
