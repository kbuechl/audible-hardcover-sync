export const inputClass = 'w-full text-base sm:text-sm px-3.5 py-2.5 border-[1.5px] border-line focus:border-pine rounded-lg bg-paper outline-none'

export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}
