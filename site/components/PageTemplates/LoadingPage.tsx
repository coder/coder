import { makeStyles } from "@material-ui/core/styles"
import CircularProgress from "@material-ui/core/CircularProgress"
import React from "react"
import { RequestState } from "../../hooks/useRequestor"

export interface LoadingPageProps<T> {
  request: RequestState<T>
  // Render Prop pattern: https://reactjs.org/docs/render-props.html
  render: (state: T) => React.ReactElement<any, any>
}

const useStyles = makeStyles(() => ({
  fullScreenLoader: {
    position: "absolute",
    top: "0",
    left: "0",
    right: "0",
    bottom: "0",
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },
}))

/**
 * `<LoadingPage />` is a helper component that manages loading state when making requests.
 *
 * While a request is in-flight, the component will show a loading spinner.
 * If a request fails, an error display will show.
 * Finally, if the request succeeds, we use the Render Prop pattern to pass it off:
 * https://reactjs.org/docs/render-props.html
 */
export const LoadingPage = <T,>(props: LoadingPageProps<T>): React.ReactElement<any, any> => {
  const styles = useStyles()

  const { request, render } = props
  const { state } = request

  switch (state) {
    case "error":
      return <div className={styles.fullScreenLoader}>{request.error.toString()}</div>
    case "loading":
      return (
        <div className={styles.fullScreenLoader}>
          <CircularProgress />
        </div>
      )
    case "success":
      return render(request.payload)
  }
}
