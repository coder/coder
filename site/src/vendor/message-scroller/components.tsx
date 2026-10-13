// Vendored from @shadcn/react 0.3.0 (MIT; see LICENSE in site/src/vendor).
// Upstream: https://github.com/shadcn-ui/ui/tree/b1c580c/packages/react/src/message-scroller
// Local deviations must be marked LOCAL CHANGE.

import * as React from "react"

import { composeRefs, mergeProps, useRender } from "../use-render"
import { USER_SCROLL_KEYS } from "./types"
import type {
  MessageScrollerButtonProps,
  MessageScrollerContentProps,
  MessageScrollerContextValue,
  MessageScrollerItemProps,
  MessageScrollerProps,
  MessageScrollerProviderProps,
  MessageScrollerRegisterMessage,
  MessageScrollerViewportProps,
} from "./types"
import { useMessageScrollerController } from "./use-message-scroller-controller"
import { useLatest } from "./utils"

const MessageScrollerContext =
  React.createContext<MessageScrollerContextValue | null>(null)
const MessageScrollerItemContext =
  React.createContext<MessageScrollerRegisterMessage | null>(null)
// LOCAL CHANGE: a no-op outside a Viewport, so tool rows also render
// standalone.
const MessageScrollerUserLayoutIntentContext = React.createContext<
  (target: Element) => void
>(() => {})

// LOCAL CHANGE
/**
 * Returns the callback for a user's expand or collapse. Pass the toggled
 * element, because a toggle above the anchored turn releases the anchor. It
 * stops following new output, so do not use it for other layout changes.
 */
function useMessageScrollerUserLayoutIntent() {
  return React.useContext(MessageScrollerUserLayoutIntentContext)
}

function useMessageScrollerContext() {
  const context = React.useContext(MessageScrollerContext)

  if (!context) {
    throw new Error("useMessageScroller must be used within a MessageScroller.")
  }

  return context
}

function useMessageScrollerItemContext() {
  const context = React.useContext(MessageScrollerItemContext)

  if (!context) {
    throw new Error(
      "MessageScrollerItem must be used within a MessageScroller."
    )
  }

  return context
}

function useMessageScroller() {
  const { scrollToEnd, scrollToMessage, scrollToStart } =
    useMessageScrollerContext()

  return React.useMemo(
    () => ({
      scrollToEnd,
      scrollToMessage,
      scrollToStart,
    }),
    [scrollToEnd, scrollToMessage, scrollToStart]
  )
}

function useMessageScrollerScrollable() {
  const { stateStore } = useMessageScrollerContext()

  return React.useSyncExternalStore(
    stateStore.subscribe,
    stateStore.getSnapshot,
    stateStore.getSnapshot
  )
}

function useMessageScrollerVisibility() {
  const { observeVisibility, unobserveVisibility, visibilityStore } =
    useMessageScrollerContext()
  const subscribe = React.useCallback(
    (listener: () => void) =>
      visibilityStore.subscribe(
        listener,
        observeVisibility,
        unobserveVisibility
      ),
    [observeVisibility, unobserveVisibility, visibilityStore]
  )

  return React.useSyncExternalStore(
    subscribe,
    visibilityStore.getSnapshot,
    visibilityStore.getSnapshot
  )
}

function MessageScrollerProvider({
  autoScroll = false,
  children,
  defaultScrollPosition = "end",
  scrollEdgeThreshold,
  scrollPreviousItemPeek,
  scrollMargin,
}: MessageScrollerProviderProps) {
  const { context, registerMessage } = useMessageScrollerController({
    autoScroll,
    defaultScrollPosition,
    scrollEdgeThreshold,
    scrollPreviousItemPeek,
    scrollMargin,
  })

  return (
    <MessageScrollerContext.Provider value={context}>
      <MessageScrollerItemContext.Provider value={registerMessage}>
        {children}
      </MessageScrollerItemContext.Provider>
    </MessageScrollerContext.Provider>
  )
}

function MessageScroller({ children, ...props }: MessageScrollerProps) {
  const { setRootElement } = useMessageScrollerContext()

  return (
    <div ref={setRootElement} {...props}>
      {children}
    </div>
  )
}

// LOCAL CHANGE
const TOWARD_END_KEYS = new Set(["ArrowDown", "End", "PageDown", " "])

// LOCAL CHANGE: an overlay scrollbar takes no width, so a strip this wide at
// the right edge stands in for its track.
const OVERLAY_SCROLLBAR_WIDTH = 16

// LOCAL CHANGE: a scrollbar press targets the viewport itself, past
// clientWidth for a classic scrollbar. The press counts even if the thumb
// does not move, and so does a press on blank space in the overlay strip.
function isScrollbarPress(event: React.PointerEvent<HTMLDivElement>) {
  const viewport = event.currentTarget
  const { clientHeight, clientWidth, offsetWidth, scrollHeight } = viewport
  const trackStart =
    clientWidth < offsetWidth
      ? clientWidth
      : offsetWidth - OVERLAY_SCROLLBAR_WIDTH

  return (
    event.target === viewport &&
    scrollHeight > clientHeight &&
    event.nativeEvent.offsetX >= trackStart
  )
}

function MessageScrollerViewport({
  "aria-label": ariaLabel,
  children,
  onKeyDown,
  onPointerDown, // LOCAL CHANGE
  onScroll,
  onTouchMove,
  onTouchStart, // LOCAL CHANGE
  onWheel,
  preserveScrollOnPrepend = true,
  ref,
  role,
  tabIndex,
  ...props
}: MessageScrollerViewportProps) {
  const {
    handleResize,
    preserveScrollOnPrependRef,
    resetBrowserScrollAnchor, // LOCAL CHANGE
    setViewportElement,
    stateStore, // LOCAL CHANGE
    syncAfterScroll,
    syncAfterScrollbarPress, // LOCAL CHANGE
    // LOCAL CHANGE
    userLayoutIntent,
    userScrollIntent,
    viewportRef,
  } = useMessageScrollerContext()
  const touchStartYRef = React.useRef(-Infinity) // LOCAL CHANGE

  preserveScrollOnPrependRef.current = preserveScrollOnPrepend

  const setViewportRef = React.useCallback(
    (element: HTMLDivElement | null) => {
      setViewportElement(element)
      composeRefs(ref)?.(element)
    },
    [ref, setViewportElement]
  )

  function handleScroll(event: React.UIEvent<HTMLDivElement>) {
    syncAfterScroll()
    onScroll?.(event)
  }

  // LOCAL CHANGE: a scrollbar drag fires no wheel, touch, or key event. A
  // press at the end may start a drag up, so it syncs only once released.
  function handlePointerDown(event: React.PointerEvent<HTMLDivElement>) {
    if (
      isScrollbarPress(event) &&
      userScrollIntent() &&
      !stateStore.getSnapshot().end
    ) {
      const viewport = event.currentTarget
      const pressScrollTop = viewport.scrollTop
      // A touch that turns into a pan ends with pointercancel, not pointerup,
      // and a release outside the window may never arrive. Drop the release
      // then, so a later click cannot trigger it.
      const press = new AbortController()
      const drop = () => press.abort()

      viewport.addEventListener(
        "pointerup",
        () => syncAfterScrollbarPress(pressScrollTop),
        { once: true, signal: press.signal }
      )
      viewport.addEventListener("pointercancel", drop, { signal: press.signal })
      viewport.addEventListener("pointerdown", drop, { signal: press.signal })
    }

    onPointerDown?.(event)
  }

  function handleWheel(event: React.WheelEvent<HTMLDivElement>) {
    // LOCAL CHANGE
    if (userScrollIntent() && event.deltaY > 0) {
      syncAfterScroll()
    }
    onWheel?.(event)
  }

  function handleTouchStart(event: React.TouchEvent<HTMLDivElement>) {
    // LOCAL CHANGE
    touchStartYRef.current = event.touches[0]?.clientY ?? -Infinity
    onTouchStart?.(event)
  }

  function handleTouchMove(event: React.TouchEvent<HTMLDivElement>) {
    // LOCAL CHANGE: a finger moving up pans toward the end, so at the end it
    // fires no scroll event, as a wheel down does.
    const touchY = event.touches[0]?.clientY
    if (userScrollIntent() && touchY < touchStartYRef.current) {
      syncAfterScroll()
    }
    onTouchMove?.(event)
  }

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    // LOCAL CHANGE: Space on a button activates it instead of scrolling.
    const activatesButton =
      event.key === " " && event.target instanceof HTMLButtonElement

    if (USER_SCROLL_KEYS.has(event.key) && !activatesButton) {
      // LOCAL CHANGE
      if (
        userScrollIntent() &&
        TOWARD_END_KEYS.has(event.key) &&
        !(event.key === " " && event.shiftKey)
      ) {
        syncAfterScroll()
      }
    }

    onKeyDown?.(event)
  }

  React.useEffect(() => {
    const viewport = viewportRef.current

    if (!viewport || typeof ResizeObserver === "undefined") {
      return
    }

    // Coalesce into rAF: handleResize mutates the spacer inside the observed
    // content, and resizing an observed element during delivery fires
    // "ResizeObserver loop completed with undelivered notifications".
    let frame = 0

    const observer = new ResizeObserver(() => {
      // LOCAL CHANGE: not in the rAF, so output before the next frame cannot
      // scroll a clamped view back down. A scroll resizes nothing.
      resetBrowserScrollAnchor()
      window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(handleResize)
    })

    observer.observe(viewport)

    return () => {
      window.cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [handleResize, resetBrowserScrollAnchor, viewportRef]) // LOCAL CHANGE

  return (
    // LOCAL CHANGE: wraps the upstream div.
    <MessageScrollerUserLayoutIntentContext.Provider value={userLayoutIntent}>
      <div
        ref={setViewportRef}
        role={role ?? "region"}
        aria-label={ariaLabel ?? "Messages"}
        tabIndex={tabIndex ?? 0}
        onKeyDown={handleKeyDown}
        onPointerDown={handlePointerDown} // LOCAL CHANGE
        onScroll={handleScroll}
        onTouchMove={handleTouchMove}
        onTouchStart={handleTouchStart} // LOCAL CHANGE
        onWheel={handleWheel}
        {...props}
      >
        {children}
      </div>
    </MessageScrollerUserLayoutIntentContext.Provider>
  )
}

function MessageScrollerContent({
  "aria-relevant": ariaRelevant,
  children,
  ref,
  role,
  spacerClassName,
  ...props
}: MessageScrollerContentProps) {
  const {
    handleContentChange,
    handleResize,
    resetBrowserScrollAnchor, // LOCAL CHANGE
    setContentElement,
    setSpacerElement,
  } = useMessageScrollerContext()
  const contentRef = React.useRef<HTMLDivElement | null>(null)

  const setContentRef = React.useCallback(
    (element: HTMLDivElement | null) => {
      contentRef.current = element
      setContentElement(element)
      composeRefs(ref)?.(element)
    },
    [ref, setContentElement]
  )

  React.useLayoutEffect(() => {
    const content = contentRef.current

    if (!content) {
      return
    }

    handleContentChange()

    if (typeof MutationObserver === "undefined") {
      return
    }

    const observer = new MutationObserver(() => {
      handleContentChange()
    })

    observer.observe(content, { childList: true })

    return () => observer.disconnect()
  }, [handleContentChange])

  React.useEffect(() => {
    const content = contentRef.current

    if (!content || typeof ResizeObserver === "undefined") {
      return
    }

    // Coalesce into rAF: handleResize mutates the spacer inside this observed
    // element, and resizing an observed element during delivery fires
    // "ResizeObserver loop completed with undelivered notifications".
    let frame = 0

    const observer = new ResizeObserver(() => {
      resetBrowserScrollAnchor() // LOCAL CHANGE: as in MessageScrollerViewport.
      window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(handleResize)
    })

    observer.observe(content)

    return () => {
      window.cancelAnimationFrame(frame)
      observer.disconnect()
    }
  }, [handleResize, resetBrowserScrollAnchor]) // LOCAL CHANGE

  return (
    <div
      ref={setContentRef}
      role={role ?? "log"}
      aria-relevant={ariaRelevant ?? "additions"}
      {...props}
    >
      {children}
      <div
        ref={setSpacerElement}
        aria-hidden="true"
        data-message-scroller-spacer=""
        hidden
        className={spacerClassName}
      />
    </div>
  )
}

function MessageScrollerItem({
  messageId,
  ref,
  scrollAnchor = false,
  ...props
}: MessageScrollerItemProps) {
  const registerMessage = useMessageScrollerItemContext()
  const elementRef = React.useRef<HTMLDivElement | null>(null)

  const setItemRef = React.useCallback(
    (element: HTMLDivElement | null) => {
      const previousElement = elementRef.current

      elementRef.current = element

      if (messageId) {
        registerMessage(messageId, element, previousElement)
      }

      composeRefs(ref)?.(element)
    },
    [messageId, ref, registerMessage]
  )

  return (
    <div
      ref={setItemRef}
      data-message-id={messageId}
      data-scroll-anchor={scrollAnchor ? "true" : "false"}
      {...props}
    />
  )
}

function MessageScrollerButton({
  behavior = "smooth",
  children,
  direction = "end",
  onClick,
  render,
  tabIndex,
  type = "button",
  ...props
}: MessageScrollerButtonProps) {
  const { scrollToEnd, scrollToStart, stateStore } = useMessageScrollerContext()
  const onClickRef = useLatest(onClick)
  const subscribe = React.useCallback(
    (listener: () => void) => stateStore.subscribe(listener),
    [stateStore]
  )
  const getSnapshot = React.useCallback(() => {
    const state = stateStore.getSnapshot()

    return direction === "start" ? state.start : state.end
  }, [direction, stateStore])
  const isActive = React.useSyncExternalStore(
    subscribe,
    getSnapshot,
    getSnapshot
  )

  const handleClick = React.useCallback(
    (event: React.MouseEvent<HTMLButtonElement>) => {
      if (!isActive) {
        return
      }

      onClickRef.current?.(event)

      if (!event.defaultPrevented) {
        event.currentTarget.blur()

        if (direction === "start") {
          scrollToStart({ behavior })
        } else {
          scrollToEnd({ behavior })
        }
      }
    },
    [behavior, direction, isActive, onClickRef, scrollToEnd, scrollToStart]
  )

  return useRender({
    defaultTagName: "button",
    props: mergeProps<"button">(
      {
        type,
        inert: !isActive,
        tabIndex: isActive ? tabIndex : -1,
        children: children ?? <span>Scroll to {direction}</span>,
        onClick: handleClick,
      },
      props
    ),
    render,
    state: {
      active: isActive,
      direction,
    },
    stateAttributesMapping: {
      active: (value) => ({
        "data-active": value ? "true" : "false",
      }),
    },
  })
}

export {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
  useMessageScroller,
  // LOCAL CHANGE
  useMessageScrollerUserLayoutIntent,
  useMessageScrollerScrollable,
  useMessageScrollerVisibility,
}
