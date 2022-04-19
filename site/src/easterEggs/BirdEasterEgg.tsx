import React, { useEffect, useState } from "react"

export const BirdEasterEgg: React.FC = () => {
  const [showBird, setShowBird] = useState<boolean>(false)

  useEffect(() => {
    const area = 200
    const xPoint = window.innerWidth - area
    const yPoint = window.innerHeight - area

    window.addEventListener("mousemove", (event) => {
      const isMouseInTheArea = event.clientX > xPoint && event.clientY > yPoint
      setShowBird(isMouseInTheArea)
    })
  }, [])

  return (
    <div
      style={{
        position: "fixed",
        bottom: 0,
        right: 20,
        transition: "all 0.25s ease-in-out",
        transform: showBird ? "translateY(50px)" : "translateY(200px)",
        display: "flex",
        flexDirection: "column",
        alignItems: "end"
      }}
    >
      <div style={{ opacity: showBird ? 1 : 0, transition: "all 0.25s 0.25s ease-in-out", padding: 20, backgroundColor: "#FFF", borderRadius: 8, width:300, lineHeight: 1.2, border: "2px solid #CCC", position: "relative", top: -8 }}>
        <div style={{ fontWeight: 700, fontSize: 18, marginBottom: 4 }}>Hey, I'm Blue, do you need help to start with Coder?</div>
        <div style={{ marginBottom: 12 }}>Let's create your first workspace together using the CLI</div>
        <button>Let's go</button>
      </div>

      <svg width="162" height="181" viewBox="0 0 162 181" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path
          d="M52.8733 113.58C54.5595 127.752 53.3896 140.806 50.2198 150.429C46.994 160.222 41.9851 165.731 36.497 166.384C31.0088 167.037 24.8471 162.856 19.4136 154.094C14.0745 145.483 9.8746 133.069 8.18841 118.896C6.50223 104.723 7.67211 91.6699 10.8419 82.0472C14.0677 72.2542 19.0765 66.7445 24.5647 66.0915C30.0529 65.4386 36.2146 69.6193 41.6481 78.382C46.9871 86.9924 51.1871 99.407 52.8733 113.58Z"
          fill="#147E84"
          stroke="black"
          strokeWidth="4"
        />
        <path
          d="M108.219 113.58C106.533 127.752 107.703 140.806 110.873 150.429C114.099 160.222 119.107 165.731 124.596 166.384C130.084 167.037 136.245 162.856 141.679 154.094C147.018 145.483 151.218 133.069 152.904 118.896C154.59 104.723 153.42 91.6699 150.251 82.0472C147.025 72.2542 142.016 66.7445 136.528 66.0915C131.04 65.4386 124.878 69.6193 119.444 78.382C114.105 86.9924 109.905 99.407 108.219 113.58Z"
          fill="#147E84"
          stroke="black"
          strokeWidth="4"
        />
        <rect
          x="21.0309"
          y="2"
          width="116"
          height="177"
          rx="58"
          fill="url(#paint0_linear_4_32)"
          stroke="black"
          strokeWidth="4"
        />
        <circle cx="42.0309" cy="52" r="6" fill="black" />
        <circle cx="43.3642" cy="49.7778" r="1.55556" fill="white" />
        <circle cx="116.031" cy="52" r="6" fill="black" />
        <circle cx="117.364" cy="49.7778" r="1.55556" fill="white" />
        <path
          d="M100.531 64C100.531 73.1229 97.9725 81.3167 93.9139 87.1888C89.8511 93.0669 84.3877 96.5 78.5309 96.5C72.674 96.5 67.2106 93.0669 63.1478 87.1888C59.0892 81.3167 56.5309 73.1229 56.5309 64C56.5309 54.8771 59.0892 46.6833 63.1478 40.8112C67.2106 34.9331 72.674 31.5 78.5309 31.5C84.3877 31.5 89.8511 34.9331 93.9139 40.8112C97.9725 46.6833 100.531 54.8771 100.531 64Z"
          fill="#AA3A3A"
          stroke="black"
          strokeWidth="3"
        />
        <path
          fillRule="evenodd"
          clipRule="evenodd"
          d="M56.0309 52.8039L78.227 75L100.423 52.8039C97.2239 39.5268 88.4944 30 78.227 30C67.9595 30 59.23 39.5268 56.0309 52.8039Z"
          fill="#E54646"
        />
        <path
          d="M78.227 75L77.1663 76.0607L78.227 77.1213L79.2876 76.0607L78.227 75ZM56.0309 52.8039L54.5726 52.4525L54.3756 53.27L54.9702 53.8646L56.0309 52.8039ZM100.423 52.8039L101.484 53.8646L102.078 53.27L101.881 52.4525L100.423 52.8039ZM79.2876 73.9393L57.0915 51.7432L54.9702 53.8646L77.1663 76.0607L79.2876 73.9393ZM99.3624 51.7432L77.1663 73.9393L79.2876 76.0607L101.484 53.8646L99.3624 51.7432ZM78.227 31.5C87.4768 31.5 95.8371 40.1745 98.9648 53.1553L101.881 52.4525C98.6108 38.879 89.512 28.5 78.227 28.5V31.5ZM57.4891 53.1553C60.6168 40.1745 68.9771 31.5 78.227 31.5V28.5C66.9419 28.5 57.8431 38.879 54.5726 52.4525L57.4891 53.1553Z"
          fill="black"
        />
        <defs>
          <linearGradient
            id="paint0_linear_4_32"
            x1="79.0309"
            y1="103"
            x2="79.0309"
            y2="181"
            gradientUnits="userSpaceOnUse"
          >
            <stop stopColor="#50DEE7" />
            <stop offset="1" stopColor="#2C91A7" />
          </linearGradient>
        </defs>
      </svg>
    </div>
  )
}
