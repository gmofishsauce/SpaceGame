# SpaceGame Vision Statement

I am "Jeff" the user/developer/evaluator on this host. This is my vision for the game located in this repo, currently called "SpaceGame" in lieu of a catchier eventual name.

SpaceGame is a real time simulation intended to proceed at a measured pace between a human player and a bult-in bot opponent with each game taking 2 to 4 hours to play. The game leverages well-known SF tropes taken from classic novels like Haldeman's "Forever War" and works by Niven and Pournelle.  FTL travel is not a part of this game universe, rather, an efficient non-Newtonian space drive has been developed that permits high sublight speeds up to about 90% of c: think something similar to the Kzin gravity polarizer drive found in the works of Larry Niven.

Using this drive, humankind and its helpful AI assistants have spread out to the nearest stars.  Then came the first attacks from the aliens and humans found themselves in an interstellar war. Gameplay consists of actions designed to win the war by resisting the advances of the aliens. The location of the alien homeworld, or their presumed "empire" of planets, is not known, so the idea of large scale offensive operations against the alien empire does not play a role in the game. The aliens do not appear to have FTL either, yet their attacks have come at varying points on the boundaries of the rough sphere of human worlds. Human's AIs interpret this as an effort to camouflage the direction and distance of their home star systems.

Since the most rapid transfer of information possible is by vehicles using the relativistic sublight drive or using massive lasers/masers, all information about war status is long out of date when received, and similar delays are required to communicate orders to different worlds.  Game play therefore resembles limited-information chess.

## Game Actions

Game actions consist of translating economic output into weapons construction and deployments and ordering existing forces to move from place to place. There are no economic actions in the gameplay; the economic power to produce weapons simply grows over time in each star systemas a function of the population and natural resources of star systems, which are randomly initialized before the game begins. Each star system's economy produces value which can be translated into weapons through game actions, after which those weapons that can move from system to system (some weapons systems are fixed in their locations) may be directed to move. The game is not turn-based, but rather the possible actions are limited by economic output. Within that limitation, any number of actions may be taken at any time by both the human and the bot.

## Combat

Combat occurs when alien and human forces find themselves in the same star system. There is no explicit combat in the gameplay. Combat is resolved by the game engine as a stocastic function of the types and quantities of weapons present when it occurs. Combat is not necessarily observable outside the star system in which it occurs.  It will be reported back to players if the forces were accompanied by the appropriate machines to report the occurrence of the combat and, if possible, its results. The presence of such machines requires explicit game actions.

## Victory conditions

Alien attacks begin to tail off after a period of play measured in the game universe, finally diminishing to none at end of play. This represents exhaustion on the part of the alien empire. Large alien losses accelerate the exhaustion point. The human side wins by minimizing losses prior to alien exhaustion and by retaining control of its star systems. The alien side wins by capturing a sufficient number (most of the...) human systems. Numerical details of these conditions will be determined from game play and folded back into the software.

## Software

The game software will be as a single page application served by a small purpose-built Golang server that only serves up the page on the local host. Human space will be visualized as in the prototype found in the directory `Proto`. The prototype will be extended to display additional game state information in addition to the name of the star system in response to mouse over events. A sidebar will be added to display events that have occurred in the recent past and a way for the user to input their game actions (e.g. menus or text) will be designed. A well designed API for bots will allow substitution of bots in the future.



