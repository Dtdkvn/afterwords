export function parseReminderOffsets(value: string): number[] {
  if (!value.trim()) return []
  const parsed = value.split(',').map((part) => Number(part.trim()))
  if (parsed.some((day) => !Number.isInteger(day) || day < 0 || day > 365) || new Set(parsed).size !== parsed.length) {
    throw new Error('Reminder days must be unique whole numbers from 0 to 365, separated by commas.')
  }
  return parsed
}

export function formatReminderOffsets(values: number[]): string { return values.join(', ') }
