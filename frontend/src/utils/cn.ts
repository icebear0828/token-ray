export type ClassValue =
  | string
  | number
  | boolean
  | null
  | undefined
  | ClassValue[]
  | { readonly [key: string]: boolean | null | undefined };

export function cn(...inputs: ClassValue[]): string {
  return inputs
    .flatMap((value) =>
      !value || typeof value === "boolean"
        ? []
        : typeof value === "string" || typeof value === "number"
          ? [String(value)]
          : Array.isArray(value)
            ? [cn(...value)]
            : Object.keys(value).filter((key) => value[key])
    )
    .filter(Boolean)
    .join(" ");
}
