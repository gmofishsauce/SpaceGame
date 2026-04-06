A limited information strategy game using science fiction tropes.
The vision statement and requirements document have and have been moved to the OLD_SPECS directory.
They are still useful for learning the general concept of the game,
but the design has evolved.
See `./OLD_SPECS/VisionStatement.md` and `./OLD_SPECS/requirements.md`.

Workflow going forward: whenever the frontend source changes, run scripts/build-frontend.sh and commit the
updated web/dist/ alongside the source changes. Anyone cloning the repo can then just go build
./srv/cmd/spacegame with no npm required.
To build the server, go build -o spacegame srv/cmd/spacegame/main.go
on Windows, go build -o spacegame.exe ...
Then run the server
and visit http:localhost:8080
