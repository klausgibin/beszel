export interface HistoryConfig {
	intervalSeconds: number
	retentionDays: number
}

export interface HistoryPoint {
	time: string
	count: number
	min: number | null
	max: number | null
	mean: number | null
	coverage: number
}

export interface HistorySummary {
	count: number
	min: number | null
	max: number | null
	mean: number | null
	p50: number | null
	p95: number | null
	p99: number | null
	coverage: number
	aboveThresholdPercent: number
}

export interface HistoryResponse extends HistoryConfig {
	bucketSeconds: number
	containers: string[]
	points: HistoryPoint[]
	summary: HistorySummary & { histogram: { min: number; max: number; count: number }[] }
	daily?: (HistorySummary & { date: string })[]
}

/** datetime-local values represent the user's local timezone, not UTC. */
export function localDateTime(date: Date): string {
	const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
	return local.toISOString().slice(0, 19)
}

export function validateHistoryRange(start: string, end: string, retentionDays: number, now = Date.now()): boolean {
	const startMs = new Date(start).getTime()
	const endMs = new Date(end).getTime()
	return (
		Number.isFinite(startMs) &&
		Number.isFinite(endMs) &&
		startMs < endMs &&
		endMs <= now + 60_000 &&
		startMs >= now - retentionDays * 86_400_000 - 60_000
	)
}

/** Shift calendar days rather than fixed hours so local daylight-saving transitions remain correct. */
export function shiftHistoryRange(start: string, end: string, days: number): [string, string] {
	return [start, end].map((value) => {
		const date = new Date(value)
		date.setDate(date.getDate() + days)
		return localDateTime(date)
	}) as [string, string]
}
