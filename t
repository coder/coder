usage: git tag [-a | -s | -u <key-id>] [-f] [-m <msg> | -F <file>]
		<tagname> [<head>]
   or: git tag -d <tagname>...
   or: git tag -l [-n[<num>]] [--contains <commit>] [--no-contains <commit>] [--points-at <object>]
		[--format=<format>] [--[no-]merged [<commit>]] [<pattern>...]
   or: git tag -v [--format=<format>] <tagname>...

    -l, --list            list tag names
    -n[<n>]               print <n> lines of each tag message
    -d, --delete          delete tags
    -v, --verify          verify tags

Tag creation options
    -a, --annotate        annotated tag, needs a message
    -m, --message <message>
                          tag message
    -F, --file <file>     read message from file
    -e, --edit            force edit of tag message
    -s, --sign            annotated and GPG-signed tag
    --cleanup <mode>      how to strip spaces and #comments from message
    -u, --local-user <key-id>
                          use another key to sign the tag
    -f, --force           replace the tag if exists
    --create-reflog       create a reflog

Tag listing options
    --column[=<style>]    show tag list in columns
    --contains <commit>   print only tags that contain the commit
    --no-contains <commit>
                          print only tags that don't contain the commit
    --merged <commit>     print only tags that are merged
    --no-merged <commit>  print only tags that are not merged
    --sort <key>          field name to sort on
    --points-at <object>  print only tags of the object
    --format <format>     format to use for the output
    --color[=<when>]      respect format colors
    -i, --ignore-case     sorting and filtering are case insensitive

