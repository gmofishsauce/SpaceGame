ROOT=~/go/src/github.com/gmofishsauce/SpaceGame
SC="$ROOT/scripts"

cd $ROOT
if  ! scripts/build-frontend.sh; then
    echo FAILED
    exit 1
fi

echo go build -o spacegame ./srv/cmd/spacegame/

if ! go build -o spacegame ./srv/cmd/spacegame/; then
    echo FAILED
    exit 1
fi

echo  Build succeeded
exit 0
