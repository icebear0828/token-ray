import { useEffect, useMemo, useState } from "react";

/**
 * Keeps long tables/lists to a bounded number of rows while preserving the
 * full dataset for counts and filtering. Page resets whenever resetKey
 * changes (search, sort, or filter) and clamps when the dataset shrinks.
 */
export function usePagination<T>(items: T[], pageSize: number, resetKey?: unknown) {
  const [page, setPage] = useState(0);
  const pageCount = Math.max(1, Math.ceil(items.length / pageSize));

  useEffect(() => {
    setPage(0);
  }, [resetKey]);

  useEffect(() => {
    // Functional update so this cannot race the reset effect: a stale `page`
    // closure would otherwise win when resetKey and pageCount change together.
    setPage((current) => Math.min(current, pageCount - 1));
  }, [pageCount]);

  const safePage = Math.min(page, pageCount - 1);
  const pageItems = useMemo(
    () => items.slice(safePage * pageSize, (safePage + 1) * pageSize),
    [items, safePage, pageSize]
  );

  return { page: safePage, setPage, pageCount, pageItems, total: items.length };
}
