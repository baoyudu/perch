# Renders README media from the fabricated demo home (docs/demo/setup.sh).
#   bash docs/demo/setup.sh && vhs docs/demo/demo.tape
Output docs/demo/demo.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 18
Set Width 1600
Set Height 640
Set Padding 16
Set Theme "Catppuccin Mocha"
Set TypingSpeed 70ms
Set Framerate 30

Hide
Type `export HOME=/tmp/perch-demo/home PERCH_CONFIG_DIR=/tmp/perch-demo/config PERCH_CACHE_DIR=/tmp/perch-demo/cache PS1='❯ '`
Enter
Type `eval "$(perch init bash)" && clear`
Enter
Show

Sleep 500ms
Type "p"
Sleep 600ms
Enter
Sleep 2.5s
Screenshot docs/demo/screenshot.png
Sleep 700ms

# fuzzy filter
Type "eval"
Sleep 1.5s
Ctrl+U
Sleep 800ms

# navigate
Down
Down
Sleep 1s

# tab cycles the agent scope
Tab
Sleep 1.3s
Tab
Sleep 1.3s
Tab
Sleep 800ms

# focus + scroll the preview pane
Right
Sleep 900ms
Down
Down
Sleep 1.2s
Left
Sleep 600ms

# settings page
Ctrl+E
Sleep 1.8s
Escape
Sleep 700ms

# pick a project (cd)
Enter
Sleep 1.2s
