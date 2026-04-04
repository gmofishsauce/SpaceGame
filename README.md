# SpaceGame
A limited information strategy game using science fiction tropes.
The vision statement and requirements document have become obsolete
and have been moved to the obsolete directory. Tthey are still useful for human readers.
See `./OBSOLETE/VisionStatement.md` and `./OBSOLETE/requirements.md`.

Workflow going forward: whenever the frontend source changes, run scripts/build-frontend.sh and commit the
updated web/dist/ alongside the source changes. Anyone cloning the repo can then just go build
./srv/cmd/spacegame with no npm required.
