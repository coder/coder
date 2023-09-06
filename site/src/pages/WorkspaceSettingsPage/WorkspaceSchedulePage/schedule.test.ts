import { emptySchedule, scheduleChanged } from "./schedule"
import { emptyTTL } from "./ttl"

describe("scheduleChanged", () => {
  describe("autostart", () => {
    it("should be true if toggle values are different", () => {
      const autostart = { autostartEnabled: true, ...emptySchedule }
      const formValues = {
        autostartEnabled: false,
        ...emptySchedule,
        autostopEnabled: false,
        ttl: emptyTTL,
      }
      expect(scheduleChanged(autostart, formValues)).toBe(true)
    })
    it("should be true if schedule values are different", () => {
      const autostart = { autostartEnabled: true, ...emptySchedule }
      const formValues = {
        autostartEnabled: true,
        ...{ ...emptySchedule, monday: true, startTime: "09:00" },
        autostopEnabled: false,
        ttl: emptyTTL,
      }
      expect(scheduleChanged(autostart, formValues)).toBe(true)
    })
    it("should be false if all autostart values are the same", () => {
      const autostart = { autostartEnabled: true, ...emptySchedule }
      const formValues = {
        autostartEnabled: true,
        ...emptySchedule,
        autostopEnabled: false,
        ttl: emptyTTL,
      }
      expect(scheduleChanged(autostart, formValues)).toBe(false)
    })
  })

  describe("autostop", () => {
    it("should be true if toggle values are different", () => {
      const autostop = { autostopEnabled: true, ttl: 1000 }
      const formValues = {
        autostartEnabled: false,
        ...emptySchedule,
        autostopEnabled: false,
        ttl: 1000,
      }
      expect(scheduleChanged(autostop, formValues)).toBe(true)
    })
    it("should be true if ttl values are different", () => {
      const autostop = { autostopEnabled: true, ttl: 1000 }
      const formValues = {
        autostartEnabled: false,
        ...emptySchedule,
        autostopEnabled: true,
        ttl: 2000,
      }
      expect(scheduleChanged(autostop, formValues)).toBe(true)
    })
    it("should be false if all autostop values are the same", () => {
      const autostop = { autostopEnabled: true, ttl: 1000 }
      const formValues = {
        autostartEnabled: false,
        ...emptySchedule,
        autostopEnabled: true,
        ttl: 1000,
      }
      expect(scheduleChanged(autostop, formValues)).toBe(false)
    })
  })
})
