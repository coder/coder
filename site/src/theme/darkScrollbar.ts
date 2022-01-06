// NOTE: This file is actually from Material-UI v5:
// https://github.com/mui-org/material-ui/blob/e724d98eba018e55e1a684236a2037e24bcf050c/packages/material-ui/src/darkScrollbar/index.ts
// And licensed under the terms here:
// https://github.com/mui-org/material-ui/blob/next/LICENSE

// TODO: Once we upgrade to @material-ui v5 - this can be removed,
// and we should just use @material-ui's version of this code.

// track, thumb and active are derieved from macOS 10.15.7
const scrollBar = {
  track: "#2b2b2b",
  thumb: "#6b6b6b",
  active: "#959595",
}

// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
export default function darkScrollbar(options = scrollBar) {
  return {
    scrollbarColor: `${options.thumb} ${options.track}`,
    "&::-webkit-scrollbar, & *::-webkit-scrollbar": {
      backgroundColor: options.track,
    },
    "&::-webkit-scrollbar-thumb, & *::-webkit-scrollbar-thumb": {
      borderRadius: 8,
      backgroundColor: options.thumb,
      minHeight: 24,
      border: `3px solid ${options.track}`,
    },
    "&::-webkit-scrollbar-thumb:focus, & *::-webkit-scrollbar-thumb:focus": {
      backgroundColor: options.active,
    },
    "&::-webkit-scrollbar-thumb:active, & *::-webkit-scrollbar-thumb:active": {
      backgroundColor: options.active,
    },
    "&::-webkit-scrollbar-thumb:hover, & *::-webkit-scrollbar-thumb:hover": {
      backgroundColor: options.active,
    },
    "&::-webkit-scrollbar-corner, & *::-webkit-scrollbar-corner": {
      backgroundColor: options.track,
    },
  }
}

// END Material-UI v5 code:
