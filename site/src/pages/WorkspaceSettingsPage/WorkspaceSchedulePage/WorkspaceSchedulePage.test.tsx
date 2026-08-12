import {
	formValuesToAutostartRequest,
	formValuesToTTLRequest,
} from "./formToRequest";
import { scheduleToAutostart } from "./schedule";
import { ttlMsToAutostop } from "./ttl";
import type { WorkspaceScheduleFormValues } from "./WorkspaceScheduleForm";

const validValues: WorkspaceScheduleFormValues = {
	autostartEnabled: true,
	sunday: false,
	monday: true,
	tuesday: true,
	wednesday: true,
	thursday: true,
	friday: true,
	saturday: false,
	startTime: "09:30",
	timezone: "Canada/Eastern",
	autostopEnabled: true,
	ttl: 120,
};

describe("WorkspaceSchedulePage", () => {
	describe("formValuesToAutostartRequest", () => {
		it.each([
			[
				// Empty case
				{
					autostartEnabled: false,
					sunday: false,
					monday: false,
					tuesday: false,
					wednesday: false,
					thursday: false,
					friday: false,
					saturday: false,
					startTime: "",
					timezone: "",
					autostopEnabled: false,
					ttl: 0,
				},
				{
					schedule: "",
				},
			],
			[
				// Single day
				{
					autostartEnabled: true,
					sunday: true,
					monday: false,
					tuesday: false,
					wednesday: false,
					thursday: false,
					friday: false,
					saturday: false,
					startTime: "16:20",
					timezone: "Canada/Eastern",
					autostopEnabled: true,
					ttl: 120,
				},
				{
					schedule: "CRON_TZ=Canada/Eastern 20 16 * * 0",
				},
			],
			[
				// Standard 1-5 case
				{
					autostartEnabled: true,
					sunday: false,
					monday: true,
					tuesday: true,
					wednesday: true,
					thursday: true,
					friday: true,
					saturday: false,
					startTime: "09:30",
					timezone: "America/Central",
					autostopEnabled: true,
					ttl: 120,
				},
				{
					schedule: "CRON_TZ=America/Central 30 09 * * 1-5",
				},
			],
			[
				// Everyday
				{
					autostartEnabled: true,
					sunday: true,
					monday: true,
					tuesday: true,
					wednesday: true,
					thursday: true,
					friday: true,
					saturday: true,
					startTime: "09:00",
					timezone: "",
					autostopEnabled: true,
					ttl: 60 * 8,
				},
				{
					schedule: "00 09 * * *",
				},
			],
			[
				// Mon, Wed, Fri Evenings
				{
					autostartEnabled: true,
					sunday: false,
					monday: true,
					tuesday: false,
					wednesday: true,
					thursday: false,
					friday: true,
					saturday: false,
					startTime: "16:20",
					timezone: "",
					autostopEnabled: true,
					ttl: 60 * 3,
				},
				{
					schedule: "20 16 * * 1,3,5",
				},
			],
		] as const)("formValuesToAutostartRequest(%p) return %p", (values, request) => {
			expect(formValuesToAutostartRequest(values)).toEqual(request);
		});
	});

	describe("formValuesToTTLRequest", () => {
		it.each([
			[
				// 0 case
				{
					...validValues,
					ttl: 0,
				},
				{
					ttl_ms: null,
				},
			],
			[
				// 2 Hours = 7.2e+12 case
				{
					...validValues,
					ttl: 2,
				},
				{
					ttl_ms: 7_200_000,
				},
			],
			[
				// 8 hours = 2.88e+13 case
				{
					...validValues,
					ttl: 8,
				},
				{
					ttl_ms: 28_800_000,
				},
			],
		] as const)("formValuesToTTLRequest(%p) returns %p", (values, request) => {
			expect(formValuesToTTLRequest(values)).toEqual(request);
		});
	});

	describe("scheduleToAutostart", () => {
		it.each([
			// Empty case
			[
				undefined,
				{
					autostartEnabled: false,
					sunday: false,
					monday: false,
					tuesday: false,
					wednesday: false,
					thursday: false,
					friday: false,
					saturday: false,
					startTime: "",
					timezone: "",
				},
			],

			// Basic case: 9:30 1-5 UTC
			[
				"CRON_TZ=UTC 30 9 * * 1-5",
				{
					autostartEnabled: true,
					sunday: false,
					monday: true,
					tuesday: true,
					wednesday: true,
					thursday: true,
					friday: true,
					saturday: false,
					startTime: "09:30",
					timezone: "UTC",
				},
			],

			// Complex case: 4:20 1 3-4 6 Canada/Eastern
			[
				"CRON_TZ=Canada/Eastern 20 16 * * 1,3-4,6",
				{
					autostartEnabled: true,
					sunday: false,
					monday: true,
					tuesday: false,
					wednesday: true,
					thursday: true,
					friday: false,
					saturday: true,
					startTime: "16:20",
					timezone: "Canada/Eastern",
				},
			],
		] as const)("scheduleToAutostart(%p) returns %p", (schedule, autostart) => {
			expect(scheduleToAutostart(schedule)).toEqual(autostart);
		});
	});

	describe("ttlMsToAutostop", () => {
		it.each([
			// empty case
			[undefined, { autostopEnabled: false, ttl: 0 }],
			// zero
			[0, { autostopEnabled: false, ttl: 0 }],
			// basic case
			[28_800_000, { autostopEnabled: true, ttl: 8 }],
		] as const)("ttlMsToAutostop(%p) returns %p", (ttlMs, autostop) => {
			expect(ttlMsToAutostop(ttlMs)).toEqual(autostop);
		});
	});
});
