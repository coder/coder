import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { useMachine } from "@xstate/react"
import { User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import debounce from "just-debounce-it"
import { ChangeEvent, useState } from "react"
import { searchUserMachine } from "xServices/users/searchUserXService"

export type UserAutocompleteProps = {
  value: User | null
  onChange: (user: User | null) => void
}

export const UserAutocomplete: React.FC<UserAutocompleteProps> = ({ value, onChange }) => {
  const styles = useStyles()
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false)
  const [searchState, sendSearch] = useMachine(searchUserMachine)
  const { searchResults } = searchState.context

  const handleFilterChange = debounce((event: ChangeEvent<HTMLInputElement>) => {
    sendSearch("SEARCH", { query: event.target.value })
  }, 1000)

  return (
    <Autocomplete
      value={value}
      id="user-autocomplete"
      style={{ width: 300 }}
      open={isAutocompleteOpen}
      onOpen={() => {
        setIsAutocompleteOpen(true)
      }}
      onClose={() => {
        setIsAutocompleteOpen(false)
      }}
      onChange={(event, newValue) => {
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
          margin="none"
          variant="outlined"
          placeholder="User email or username"
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
      "& .MuiInputBase-root": {
        width: 300,
        // Match button small height
        height: 36,
      },

      "& input": {
        fontSize: 14,
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
