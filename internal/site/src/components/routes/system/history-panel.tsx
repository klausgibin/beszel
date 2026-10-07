import { useEffect, useId, useMemo, useState } from "react"
import { t } from "@lingui/core/macro"
import { Trans } from "@lingui/react/macro"
import {
	Bar,
	BarChart,
	Brush,
	CartesianGrid,
	Legend,
	Line,
	LineChart,
	ResponsiveContainer,
	Tooltip,
	XAxis,
	YAxis,
} from "recharts"
import { ChevronLeftIcon, ChevronRightIcon, DownloadIcon, RefreshCwIcon } from "lucide-react"
import { pb } from "@/lib/api"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import {
	localDateTime,
	shiftHistoryRange,
	validateHistoryRange,
	type HistoryConfig,
	type HistoryResponse,
} from "./history-data"

const percent = (value: number | null | undefined) =>
	value == null ? "—" : `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })}%`

export default function HistoryPanel({ systemId }: { systemId: string }) {
	const fieldId = useId()
	const [config, setConfig] = useState<HistoryConfig>()
	const [start, setStart] = useState(() => localDateTime(new Date(Date.now() - 86_400_000)))
	const [end, setEnd] = useState(() => localDateTime(new Date()))
	const [metric, setMetric] = useState("cpu")
	const [container, setContainer] = useState("")
	const [containers, setContainers] = useState<string[]>([])
	const [threshold, setThreshold] = useState("80")
	const [request, setRequest] = useState(0)
	const [data, setData] = useState<HistoryResponse>()
	const [loading, setLoading] = useState(false)
	const [exporting, setExporting] = useState(false)
	const [error, setError] = useState("")
	const [selection, setSelection] = useState<{ startIndex?: number; endIndex?: number }>()
	const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
	const validRange = !!config && validateHistoryRange(start, end, config.retentionDays)
	const validThreshold = threshold.trim() !== "" && Number.isFinite(Number(threshold)) && Number(threshold) >= 0
	const unit = container && metric === "memory" ? "MiB" : "%"
	const formatValue = (value: number | null | undefined) =>
		value == null ? "—" : `${value.toLocaleString(undefined, { maximumFractionDigits: 2 })}${unit}`

	useEffect(() => {
		const controller = new AbortController()
		pb.send<HistoryConfig>("/api/beszel/history/config", { signal: controller.signal, requestKey: null })
			.then(setConfig)
			.catch((cause) => {
				if (!controller.signal.aborted) setError(cause.message)
			})
		return () => controller.abort()
	}, [systemId])

	useEffect(() => {
		if (!validRange || !validThreshold) {
			setData(undefined)
			setLoading(false)
			return
		}
		const controller = new AbortController()
		setLoading(true)
		setError("")
		setData(undefined)
		setSelection(undefined)
		pb.send<HistoryResponse>("/api/beszel/history", {
			query: {
				system: systemId,
				start: new Date(start).toISOString(),
				end: new Date(end).toISOString(),
				metric,
				container,
				threshold,
				points: 1200,
				timezone,
			},
			signal: controller.signal,
			requestKey: null,
		})
			.then((response) => {
				if (!controller.signal.aborted) {
					setData(response)
					setContainers(response.containers)
				}
			})
			.catch((cause) => {
				if (!controller.signal.aborted) setError(cause.message)
			})
			.finally(() => {
				if (!controller.signal.aborted) setLoading(false)
			})
		return () => controller.abort()
	}, [systemId, start, end, metric, container, threshold, request, validRange, validThreshold, timezone])

	const points = useMemo(
		() => data?.points.map((point) => ({ ...point, timestamp: new Date(point.time).getTime() })) ?? [],
		[data]
	)
	const histogram = useMemo(
		() =>
			data?.summary.histogram.map((bucket) => ({
				...bucket,
				label: `${bucket.min.toFixed(0)}–${bucket.max.toFixed(0)}${unit}`,
				share: data.summary.count ? (bucket.count / data.summary.count) * 100 : 0,
			})) ?? [],
		[data, unit]
	)

	function setPeriod(days: number) {
		const now = new Date()
		setStart(localDateTime(new Date(now.getTime() - days * 86_400_000)))
		setEnd(localDateTime(now))
	}

	function shiftDay(days: number) {
		const [nextStart, nextEnd] = shiftHistoryRange(start, end, days)
		if (config && validateHistoryRange(nextStart, nextEnd, config.retentionDays)) {
			setStart(nextStart)
			setEnd(nextEnd)
		}
	}

	function zoom() {
		if (!selection || selection.startIndex == null || selection.endIndex == null) return
		const first = points[selection.startIndex]
		const last = points[selection.endIndex]
		if (!first || !last || first.timestamp >= last.timestamp) return
		setStart(localDateTime(new Date(first.timestamp)))
		setEnd(
			localDateTime(new Date(Math.min(last.timestamp + (data?.bucketSeconds ?? 0) * 1000, new Date(end).getTime())))
		)
	}

	async function exportCsv() {
		if (!validRange || !validThreshold) return
		setExporting(true)
		setError("")
		try {
			const query = new URLSearchParams({
				system: systemId,
				start: new Date(start).toISOString(),
				end: new Date(end).toISOString(),
				metric,
				container,
				threshold,
				export: "csv",
				timezone,
			})
			const response = await fetch(pb.buildUrl(`/api/beszel/history/export?${query}`), {
				headers: { Authorization: pb.authStore.token },
			})
			if (!response.ok) throw new Error(t`Could not export historical samples`)
			const url = URL.createObjectURL(await response.blob())
			const link = document.createElement("a")
			link.href = url
			link.download = `beszel-${systemId}-${metric}-${start.slice(0, 10)}.csv`
			link.click()
			setTimeout(() => URL.revokeObjectURL(url), 1000)
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : String(cause))
		} finally {
			setExporting(false)
		}
	}

	return (
		<section className="grid gap-4" aria-label={t`Historical analysis`} aria-busy={loading}>
			<Card>
				<CardHeader>
					<CardTitle>
						<Trans>Historical analysis</Trans>
					</CardTitle>
					<CardDescription>
						<Trans>
							Original samples remain available when zooming or exporting. Overview charts preserve observed peaks.
						</Trans>
					</CardDescription>
					{container && metric === "cpu" && (
						<p className="text-sm text-muted-foreground">
							<Trans>Container CPU may exceed 100% when using multiple cores.</Trans>
						</p>
					)}
					{config && (
						<p className="text-sm text-muted-foreground">
							<Trans>
								Sampling: {config.intervalSeconds}s · Retention: {config.retentionDays} days
							</Trans>{" "}
							· {timezone}
						</p>
					)}
				</CardHeader>
				<CardContent className="grid gap-4">
					<div className="grid sm:grid-cols-2 xl:grid-cols-4 gap-3">
						<label className="grid gap-1.5 text-sm" htmlFor={`${fieldId}-start`}>
							<Trans>From</Trans>
							<Input
								id={`${fieldId}-start`}
								type="datetime-local"
								step="1"
								value={start}
								onChange={(event) => setStart(event.target.value)}
							/>
						</label>
						<label className="grid gap-1.5 text-sm" htmlFor={`${fieldId}-end`}>
							<Trans>To</Trans>
							<Input
								id={`${fieldId}-end`}
								type="datetime-local"
								step="1"
								value={end}
								onChange={(event) => setEnd(event.target.value)}
							/>
						</label>
						<div className="grid gap-1.5 text-sm">
							<label htmlFor={`${fieldId}-target`}>
								<Trans>System or container</Trans>
							</label>
							<Select
								value={container || "__host"}
								onValueChange={(value) => {
									setContainer(value === "__host" ? "" : value)
									if (metric === "disk") setMetric("cpu")
								}}
							>
								<SelectTrigger id={`${fieldId}-target`} className="min-w-0">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="__host">
										<Trans>System</Trans>
									</SelectItem>
									{[...new Set([...containers, ...(container ? [container] : [])])].map((name) => (
										<SelectItem key={name} value={name}>
											{name}
										</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-1.5 text-sm">
							<label htmlFor={`${fieldId}-metric`}>
								<Trans>Metric</Trans>
							</label>
							<Select value={metric} onValueChange={setMetric}>
								<SelectTrigger id={`${fieldId}-metric`}>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="cpu">
										<Trans>CPU Usage</Trans>
									</SelectItem>
									<SelectItem value="memory">
										<Trans>Memory Usage</Trans>
									</SelectItem>
									{!container && (
										<SelectItem value="disk">
											<Trans>Disk Usage</Trans>
										</SelectItem>
									)}
								</SelectContent>
							</Select>
						</div>
					</div>
					<div className="flex flex-wrap gap-2 items-center">
						<Button
							variant="outline"
							size="icon"
							aria-label={t`Previous day`}
							disabled={!config || !validateHistoryRange(...shiftHistoryRange(start, end, -1), config.retentionDays)}
							onClick={() => shiftDay(-1)}
						>
							<ChevronLeftIcon className="size-4" />
						</Button>
						<Button
							variant="outline"
							size="icon"
							aria-label={t`Next day`}
							disabled={!config || !validateHistoryRange(...shiftHistoryRange(start, end, 1), config.retentionDays)}
							onClick={() => shiftDay(1)}
						>
							<ChevronRightIcon className="size-4" />
						</Button>
						{[1, 7, 30].map((days) => (
							<Button
								key={days}
								variant="outline"
								disabled={!config || days > config.retentionDays}
								onClick={() => setPeriod(days)}
							>
								{days}d
							</Button>
						))}
						<Button
							variant="outline"
							disabled={!validRange || loading || !validThreshold}
							onClick={() => setRequest((value) => value + 1)}
						>
							<RefreshCwIcon className="size-4 me-2" />
							<Trans>Refresh</Trans>
						</Button>
						<Button variant="outline" disabled={!validRange || exporting || !validThreshold} onClick={exportCsv}>
							<DownloadIcon className="size-4 me-2" />
							{exporting ? t`Exporting…` : t`Export original CSV`}
						</Button>
						<label htmlFor={`${fieldId}-threshold`} className="flex gap-2 items-center text-sm sm:ms-auto">
							<Trans>Threshold</Trans>
							<Input
								id={`${fieldId}-threshold`}
								type="number"
								min="0"
								value={threshold}
								className="w-20"
								onChange={(event) => setThreshold(event.target.value)}
							/>
							{unit}
						</label>
					</div>
					{config && (!validRange || !validThreshold) && (
						<p role="alert" className="text-sm text-destructive">
							<Trans>
								Choose a valid range within retention, ending no later than now, and a nonnegative threshold.
							</Trans>
						</p>
					)}
					{error && (
						<p role="alert" className="text-sm text-destructive">
							{error}
						</p>
					)}
					{loading && (
						<output className="text-sm text-muted-foreground">
							<Trans>Loading original samples…</Trans>
						</output>
					)}
				</CardContent>
			</Card>
			{data && (
				<>
					<div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-3">
						{[
							[t`Median (p50)`, formatValue(data.summary.p50)],
							["p95", formatValue(data.summary.p95)],
							["p99", formatValue(data.summary.p99)],
							[t`Observed peak`, formatValue(data.summary.max)],
							[t`Coverage`, percent(data.summary.coverage)],
							[t`Time above threshold`, percent(data.summary.aboveThresholdPercent)],
						].map(([label, value]) => (
							<Card key={label} className="p-4">
								<p className="text-xs text-muted-foreground mb-2">{label}</p>
								<p className="font-semibold text-lg">{value}</p>
							</Card>
						))}
					</div>
					{!data.summary.count ? (
						<Card className="p-6 text-sm text-muted-foreground">
							<Trans>
								No archived samples in this range. History starts when collection is enabled; unavailable systems and
								absent containers are gaps.
							</Trans>
						</Card>
					) : (
						<>
							<Card>
								<CardHeader>
									<CardTitle>
										{metric === "cpu" ? t`CPU Usage` : metric === "memory" ? t`Memory Usage` : t`Disk Usage`} · {unit}
									</CardTitle>
									<CardDescription>
										<Trans>
											{data.summary.count} samples · Display buckets: {data.bucketSeconds}s. Missing observations are
											gaps.
										</Trans>
									</CardDescription>
								</CardHeader>
								<CardContent>
									<div
										className="h-80"
										role="img"
										aria-label={t`Historical usage chart with minimum, mean and observed peak`}
									>
										<ResponsiveContainer width="100%" height="100%">
											<LineChart data={points} margin={{ left: 0, right: 15 }}>
												<CartesianGrid stroke="var(--border)" strokeDasharray="3 3" />
												<XAxis
													dataKey="timestamp"
													type="number"
													domain={[new Date(start).getTime(), new Date(end).getTime()]}
													tickFormatter={(value) =>
														new Date(value).toLocaleString(undefined, {
															month: "short",
															day: "numeric",
															hour: "2-digit",
															minute: "2-digit",
														})
													}
													minTickGap={45}
												/>
												<YAxis tickFormatter={(value) => `${value}${unit}`} width={65} domain={[0, "auto"]} />
												<Tooltip
													labelFormatter={(value) => new Date(Number(value)).toLocaleString()}
													formatter={(value) => formatValue(Number(value))}
												/>
												<Legend />
												<Line
													type="linear"
													dataKey="max"
													name={t`Observed peak`}
													stroke="var(--chart-1)"
													strokeWidth={1.5}
													dot={false}
													connectNulls={false}
													isAnimationActive={false}
												/>
												<Line
													type="linear"
													dataKey="mean"
													name={t`Mean`}
													stroke="var(--chart-2)"
													dot={false}
													connectNulls={false}
													isAnimationActive={false}
												/>
												<Line
													type="linear"
													dataKey="min"
													name={t`Minimum`}
													stroke="var(--chart-3)"
													dot={false}
													connectNulls={false}
													isAnimationActive={false}
												/>
												<Brush
													dataKey="timestamp"
													height={25}
													travellerWidth={10}
													tickFormatter={(value) => new Date(value).toLocaleDateString()}
													onChange={setSelection}
												/>
											</LineChart>
										</ResponsiveContainer>
									</div>
									<div className="flex justify-end mt-4">
										<Button
											variant="outline"
											onClick={zoom}
											disabled={!selection || selection.startIndex === selection.endIndex}
										>
											<Trans>Zoom into selection</Trans>
										</Button>
									</div>
								</CardContent>
							</Card>
							<Card>
								<CardHeader>
									<CardTitle>
										<Trans>Usage distribution</Trans>
									</CardTitle>
									<CardDescription>
										<Trans>
											Percentage of original observations in each usage band. CPU is measured over each sampling
											interval.
										</Trans>
									</CardDescription>
								</CardHeader>
								<CardContent>
									<div className="h-64" role="img" aria-label={t`Usage distribution histogram`}>
										<ResponsiveContainer width="100%" height="100%">
											<BarChart data={histogram}>
												<CartesianGrid stroke="var(--border)" strokeDasharray="3 3" />
												<XAxis dataKey="label" minTickGap={25} />
												<YAxis tickFormatter={(value) => `${value}%`} />
												<Tooltip formatter={(value) => percent(Number(value))} />
												<Bar dataKey="share" name={t`Observations`} fill="var(--chart-1)" isAnimationActive={false} />
											</BarChart>
										</ResponsiveContainer>
									</div>
								</CardContent>
							</Card>
							{data.daily && (
								<Card>
									<CardHeader>
										<CardTitle>
											<Trans>Daily analysis</Trans>
										</CardTitle>
										<CardDescription>
											<Trans>
												Percentiles use original samples. Time above threshold is relative to observed time; coverage
												shows missing time.
											</Trans>
										</CardDescription>
									</CardHeader>
									<CardContent>
										<div className="overflow-x-auto">
											<table className="w-full text-sm text-start">
												<thead>
													<tr className="border-b text-muted-foreground">
														{[
															t`Date`,
															t`Samples`,
															"p50",
															"p95",
															"p99",
															t`Observed peak`,
															t`Coverage`,
															t`Above threshold`,
														].map((heading) => (
															<th key={heading} className="p-2 text-start font-medium">
																{heading}
															</th>
														))}
													</tr>
												</thead>
												<tbody>
													{data.daily.map((day) => (
														<tr key={day.date} className="border-b last:border-0">
															<td className="p-2 whitespace-nowrap">{day.date}</td>
															<td className="p-2">{day.count}</td>
															{[day.p50, day.p95, day.p99, day.max].map((value, index) => (
																<td key={index} className="p-2 whitespace-nowrap">
																	{formatValue(value)}
																</td>
															))}
															<td className="p-2">{percent(day.coverage)}</td>
															<td className="p-2">{percent(day.aboveThresholdPercent)}</td>
														</tr>
													))}
												</tbody>
											</table>
										</div>
									</CardContent>
								</Card>
							)}
						</>
					)}
				</>
			)}
		</section>
	)
}
