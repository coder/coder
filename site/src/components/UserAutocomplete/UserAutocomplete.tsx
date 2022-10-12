import CircularProgress from "@material-ui/core/CircularProgress"
import { makeStyles, Theme } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import Autocomplete from "@material-ui/lab/Autocomplete"
import { useMachine } from "@xstate/react"
import { User } from "api/typesGenerated"
import { AvatarData } from "components/AvatarData/AvatarData"
import debounce from "just-debounce-it"
import { ChangeEvent, FC, useEffect, useState } from "react"
import { combineClasses } from "util/combineClasses"
import { searchUserMachine } from "xServices/users/searchUserXService"
import { AutocompleteAvatar } from "./AutocompleteAvatar"

export type UserAutocompleteProps = {
  value: User | null
  onChange: (user: User | null) => void
  label?: string
  inputMargin?: "none" | "dense" | "normal"
  inputStyles?: string
  className?: string
  showAvatar?: boolean
}

export const UserAutocomplete: FC<UserAutocompleteProps> = ({
  value,
  onChange,
  className,
  label,
  inputMargin,
  inputStyles,
  showAvatar = false,
}) => {
  const styles = useStyles({ showAvatar })
  const [isAutocompleteOpen, setIsAutocompleteOpen] = useState(false)
  const [searchState, sendSearch] = useMachine(searchUserMachine)
  const { searchResults } = searchState.context

  // seed list of options on the first page load if a user pases in a value
  // since some organizations have long lists of users, we do not load all options on page load.
  useEffect(() => {
    if (value) {
      sendSearch("SEARCH", { query: value.email })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const handleFilterChange = debounce(
    (event: ChangeEvent<HTMLInputElement>) => {
      sendSearch("SEARCH", { query: event.target.value })
    },
    1000,
  )

  return (
    <Autocomplete
      value={value}
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

        onChange(newValue)
      }}
      getOptionSelected={(option: User, value: User) =>
        option.username === value.username
      }
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
      className={combineClasses([styles.autocomplete, className])}
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
            startAdornment: (
              <>{showAvatar && value && <AutocompleteAvatar user={value} />}</>
            ),
            endAdornment: (
              <>
                {searchState.matches("searching") ? (
                  <CircularProgress size={16} />
                ) : null}
                {params.InputProps.endAdornment}
              </>
            ),
          }}
        />
      )}
    />
  )
}

interface styleProps {
  showAvatar: boolean
}

export const useStyles = makeStyles<Theme, styleProps>((theme) => {
  return {
    autocomplete: (props) => ({
      width: "100%",

      "& .MuiFormControl-root": {
        width: "100%",
      },

      "& .MuiInputBase-root": {
        width: "100%",
        // Match button small height
        height: props.showAvatar ? 60 : 40,
      },

      "& input": {
        fontSize: 16,
        padding: `${theme.spacing(0, 0.5, 0, 0.5)} !important`,
      },
    }),

    avatar: {
      width: theme.spacing(4.5),
      height: theme.spacing(4.5),
      borderRadius: "100%",
    },
  }
})

export const UserAutocompleteInline: React.FC<UserAutocompleteProps> = (
  props,
) => {
  const style = useInlineStyle()

  return <UserAutocomplete {...props} className={style.inline} />
}

export const useInlineStyle = makeStyles(() => {
  return {
    inline: {
      width: "300px",

      "& .MuiFormControl-root": {
        margin: 0,
      },

      "& .MuiInputBase-root": {
        // Match button small height
        height: 36,
      },
    },
  }
})
