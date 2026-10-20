# The README recording of examples/choose. From the repository root:
#   task build && ntrecord docs/media/choose.tape
Output docs/media/choose.gif
Require sh
Set Shell /bin/sh
Set Columns 90
Set Rows 26
Set FontSize 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms
Hide
Type "clear; bin/choose -pretend"
Enter
Wait+Screen /Choose a file/
Show
Sleep 1.5s
Type "pic"
Enter
Wait+Screen /landscape/
Sleep 1s
Type "land"
Sleep 800ms
Alt+Enter
Wait+Screen /Open landscape.png with/
Sleep 2s
Tab
Wait+Screen /Edit landscape.png with/
Sleep 2s
Down
Sleep 600ms
Type "d"
Sleep 1.5s
Enter
Wait+Screen /Would edit|Would open/
Sleep 2.5s
Ctrl+C
Sleep 500ms
