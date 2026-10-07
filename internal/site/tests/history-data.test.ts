import { describe, expect, test } from "bun:test"
import { localDateTime, shiftHistoryRange, validateHistoryRange } from "../src/components/routes/system/history-data"

describe("historical range navigation", () => {
	const now = Date.parse("2026-10-07T18:00:00Z")

	test("rejects empty, reversed, future and expired ranges", () => {
		expect(validateHistoryRange("", "", 35, now)).toBe(false)
		expect(validateHistoryRange("invalid", "2026-10-07T17:00:00Z", 35, now)).toBe(false)
		expect(validateHistoryRange("2026-10-07T17:00:00Z", "2026-10-07T16:00:00Z", 35, now)).toBe(false)
		expect(validateHistoryRange("2026-10-07T17:00:00Z", "2026-10-08T00:00:00Z", 35, now)).toBe(false)
		expect(validateHistoryRange("2026-08-01T00:00:00Z", "2026-10-07T17:00:00Z", 35, now)).toBe(false)
	})

	test("allows the retention boundary and inclusive current end", () => {
		expect(validateHistoryRange("2026-09-02T18:00:00Z", "2026-10-07T18:00:00Z", 35, now)).toBe(true)
	})

	test("datetime-local conversion preserves the exact sample second", () => {
		const date = new Date("2026-10-07T17:05:15Z")
		expect(new Date(localDateTime(date)).getTime()).toBe(date.getTime())
	})

	test("day navigation preserves local time across month boundaries", () => {
		expect(shiftHistoryRange("2026-09-30T08:15:15", "2026-09-30T09:15:15", 1)).toEqual([
			"2026-10-01T08:15:15",
			"2026-10-01T09:15:15",
		])
	})
	test("day navigation preserves wall clock through daylight-saving changes", () => {
		expect(shiftHistoryRange("2026-03-07T08:15:15", "2026-03-07T09:15:15", 1)).toEqual([
			"2026-03-08T08:15:15",
			"2026-03-08T09:15:15",
		])
	})
})
