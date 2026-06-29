export type JsonAttributeConverter<T> = {
  fromAttribute(value: string | null): T
  toAttribute(value: T): string
}

export function jsonAttribute<T>(fallback: T): JsonAttributeConverter<T> {
  return {
    fromAttribute(value: string | null): T {
      if (!value) return fallback
      try {
        return JSON.parse(value) as T
      } catch {
        return fallback
      }
    },
    toAttribute(value: T): string {
      return JSON.stringify(value)
    },
  }
}
