import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { useMachine } from "@xstate/react"
import { User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import debounce from "just-debounce-it"
import { ChangeEvent, useEffect, useState } from "react"
import { searchUserMachine } from "xServices/users/searchUserXService"

export type UserAutocompleteProps = {
  value: User | null
  onChange: (user: User | null) => void
  label?: string
  inputMargin?: "none" | "dense" | "normal"
  inputStyles?: string
}

export const UserAutocomplete: React.FC<UserAutocompleteProps> = ({
  value,
  onChange,
  label,
  inputMargin,
  inputStyles,
}) => {
  const styles = useStyles()
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false)
  const [searchState, sendSearch] = useMachine(searchUserMachine)
  const { searchResults } = searchState.context
  const [selectedValue, setSelectedValue] = useState<User | null>(value || null)

  // seed list of options on the first page load if a user pases in a value
  // since some organizations have long lists of users, we do not load all options on page load.
  useEffect(() => {
    if (value) {
      sendSearch("SEARCH", { query: value.email })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleFilterChange = debounce((event: ChangeEvent<HTMLInputElement>) => {
    sendSearch("SEARCH", { query: event.target.value })
  }, 1000)

  return (
    <Autocomplete
      value={selectedValue}
      id="user-autocomplete"
      open={isAutocompleteOpen}
      onOpen={() => {
        setIsAutocompleteOpen(true)
      }}
      onClose={() => {
        setIsAutocompleteOpen(false)
      }}
      onChange={(_, newValue) => {
        if (newValue === null) {
          sendSearch("CLEAR_RESULTS")
        }

        setSelectedValue(newValue)
        onChange(newValue)
      }}
      getOptionSelected={(option: User, value: User) => option.username === value.username}
      getOptionLabel={(option) => option.email}
      renderOption={(option: User) => (
        <AvatarData
          title={option.username}
          subtitle={option.email}
          highlightTitle
          avatar={
            option.avatar_url ? (
              <img
                className={styles.avatar}
                alt={`${option.username}'s Avatar`}
                src={option.avatar_url}
              />
            ) : null
          }
        />
      )}
      options={searchResults}
      loading={searchState.matches("searching")}
      className={styles.autocomplete}
      renderInput={(params) => (
        <TextField
          {...params}
          variant="outlined"
          margin={inputMargin ?? "normal"}
          label={label ?? undefined}
          placeholder="User email or username"
          className={inputStyles}
          InputProps={{
            ...params.InputProps,
            onChange: handleFilterChange,
            endAdornment: (
              <>
                {searchState.matches("searching") ? <CircularProgress size={16} /> : null}
                {params.InputProps.endAdornment}
              </>
            ),
          }}
        />
      )}
    />
  )
}
export const useStyles = makeStyles((theme) => {
  return {
    autocomplete: {
      width: "100%",

      "& .MuiFormControl-root": {
        width: "100%",
      },

      "& .MuiInputBase-root": {
        width: "100%",
        // Match button small height
        height: 40,
      },

      "& input": {
        fontSize: 16,
        padding: `${theme.spacing(0, 0.5, 0, 0.5)} !important`,
      },
    },

    avatar: {
      width: theme.spacing(4.5),
      height: theme.spacing(4.5),
      borderRadius: "100%",
    },
  }
})
