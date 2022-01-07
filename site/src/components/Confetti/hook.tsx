import { useEffect, useRef } from "react"

interface ConfettiProps {
  frame: number
  outer: HTMLElement
  inner: HTMLElement
  axis: string
  theta: number
  dTheta: number
  x: number
  y: number
  dx: number
  dy: number
  splineX: number[]
  splineY: number[]
  update: (delta: number) => boolean
}

export const useConfetti = (element: HTMLElement | null): void => {
  const colors = ["#f54542", "#f59c42", "#75f542", "#42e9f5", "#427ef5", "#8442f5", "#cb42f5", "#f542b9"]
  const confetti: ConfettiProps[] = []
  const cosInterp = (a: number, b: number, t: number) => ((1 - Math.cos(PI * t)) / 2) * (b - a) + a
  const deviation = 100
  const dThetaMin = 0.4
  const dThetaMax = 0.7 - dThetaMin
  const dxThetaMin = -0.1
  const dxThetaMax = -dxThetaMin - dxThetaMin
  const dyMax = 0.18
  const dyMin = 0.13
  const eccentricity = 10
  const genColor = () => colors[randInt(0, colors.length - 1)]
  const PI = Math.PI
  const PI2 = PI * 2
  const radius = 1 / eccentricity
  const radius2 = radius + radius
  const randInt = (min: number, max: number) => Math.floor(Math.random() * max) + min
  const runFor = 2000
  const sizeMin = 5
  const sizeMax = 12 - sizeMin
  const spread = 20
  const frame = useRef(0)
  const isRunning = useRef<boolean>(true)
  const timer = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    isRunning.current = true
    if (element == null) {
      isRunning.current = false
      return
    }

    const rect = element.getBoundingClientRect()
    const top = rect.top
    const left = rect.left
    const winWidth = rect.width
    const winHeight = rect.height

    setTimeout(() => (isRunning.current = false), runFor)
    function createPoissonDistribution() {
      const domain = [radius, 1 - radius]
      let measure = 1 - radius2
      const spline = [0, 1]
      while (measure) {
        let dart = measure * Math.random(),
          i,
          l,
          interval,
          a,
          b

        for (i = 0, l = domain.length, measure = 0; i < l; i += 2) {
          a = domain[i]
          b = domain[i + 1]
          interval = b - a
          if (dart < measure + interval) {
            spline.push((dart += a - measure))
            break
          }
          measure += interval
        }
        const c = dart - radius
        const d = dart + radius

        for (i = domain.length - 1; i > 0; i -= 2) {
          ;(l = i - 1), (a = domain[l]), (b = domain[i])
          if (a >= c && a < d) {
            if (b > d) {
              domain[l] = d
            } else {
              domain.splice(l, 2)
            }
          } else if (a < c && b > c) {
            if (b <= d) {
              domain[i] = c
            } else {
              domain.splice(i, 0, c, d)
            }
          }
        }

        for (i = 0, l = domain.length, measure = 0; i < l; i += 2) {
          measure += domain[i + 1] - domain[i]
        }
      }

      return spline.sort()
    }

    // Create the overarching container
    const container = document.createElement("div")
    container.style.position = "absolute"
    container.style.top = `${top}px`
    container.style.left = `${left}px`
    container.style.width = `${winWidth}px`
    container.style.height = `${winHeight}px`
    container.style.overflow = "hidden"
    container.style.zIndex = "9999"
    container.style.pointerEvents = "none"

    // Confetti constructor
    class Confetti implements ConfettiProps {
      frame = 0
      outer = document.createElement("div")
      inner = document.createElement("div")
      axis = `rotate3D(${Math.cos(360 * Math.random())}, ${Math.cos(360 * Math.random())}, 0`
      theta = 360 * Math.random()
      dTheta = dThetaMin + dThetaMax * Math.random()
      x = winWidth * Math.random()
      y = -deviation
      dx = Math.sin(dxThetaMin + dxThetaMax * Math.random())
      dy = dyMin + dyMax * Math.random()
      splineX = createPoissonDistribution()
      splineY: number[] = []
      constructor() {
        this.inner.style.backgroundColor = genColor()
        this.inner.style.height = "100%"
        this.inner.style.transform = `${this.axis}, ${this.theta}deg)`
        this.inner.style.width = "100%"
        this.outer.appendChild(this.inner)
        this.outer.style.height = `${sizeMin + sizeMax * Math.random()}px`
        this.outer.style.left = `${this.x}px`
        this.outer.style.perspective = "50px"
        this.outer.style.position = "absolute"
        this.outer.style.top = `${this.y}px`
        this.outer.style.transform = `rotate(${360 * Math.random()}deg)`
        this.outer.style.width = `${sizeMin + sizeMax * Math.random()}px`

        // Create the periodic spline
        for (let i = 1, l: number = this.splineX.length - 1; i < l; ++i) {
          this.splineY[i] = deviation * Math.random()
          this.splineY[0] = this.splineY[l] = deviation * Math.random()
        }
      }

      update(delta: number) {
        this.frame += delta
        this.x += this.dx * delta
        this.y += this.dy * delta
        this.theta += this.dTheta * delta

        // Compute spline and convert to polar
        let phi = (this.frame % 7777) / 7777
        let i = 0
        let j = 1
        while (phi >= this.splineX[j]) {
          i = j++
        }
        const rho = cosInterp(
          this.splineY[i],
          this.splineY[j],
          (phi - this.splineX[i]) / (this.splineX[j] - this.splineX[i]),
        )
        phi *= PI2

        this.outer.style.left = `${this.x + rho * Math.cos(phi)}px`
        this.outer.style.top = `${this.y + rho * Math.sin(phi)}px`
        this.inner.style.transform = `${this.axis}, ${this.theta}deg)`
        return this.y > winHeight + deviation
      }
    }

    function addConfetti() {
      if (isRunning.current) {
        const confetto = new Confetti()
        confetti.push(confetto)
        container.appendChild(confetto.outer)
        timer.current = setTimeout(addConfetti, spread * Math.random())
      }
    }

    function run() {
      if (!frame.current) {
        document.body.appendChild(container)
        addConfetti()
        let prev: number
        requestAnimationFrame(function loop(timestamp: number) {
          const delta = prev ? timestamp - prev : 0
          prev = timestamp
          for (let i = confetti.length - 1; i >= 0; --i) {
            if (confetti[i].update(delta)) {
              container.removeChild(confetti[i].outer)
              confetti.splice(i, 1)
            }
          }
          if (timer.current || confetti.length) {
            return (frame.current = requestAnimationFrame(loop))
          }
          document.body.removeChild(container)
          frame.current = 0
        })
      }
    }

    run()
  }, [element])
}
