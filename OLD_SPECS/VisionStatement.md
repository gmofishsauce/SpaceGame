# SpaceGame Vision Statement

I am "Jeff" the user/developer/evaluator on this host. This is my vision for the game located in this repo, currently called "SpaceGame" in lieu of a catchier eventual name.  Many details of the gameplay will be learned from early efforts to play the game using an initial version of the software and these learnings will be folded back into the game software.

SpaceGame is a real time simulation intended to proceed at a measured pace between a human player and a bult-in bot opponent with each game taking 2 to 4 hours to play. The game leverages well-known SF tropes taken from classic novels like Haldeman's "Forever War" and works by Niven/Pournelle and others.  FTL travel is not a part of this game universe, rather, an efficient non-Newtonian space drive has been developed that permits high sublight speeds up to about 90% of c: think something similar to the Kzin gravity polarizer drive found in the works of Larry Niven.

Using this drive, humankind and its helpful AI assistants have spread out to the nearest stars.  Then came the first attacks from the aliens and humans found themselves in an interstellar war. Gameplay consists of actions designed to win the war by resisting the advances of the aliens. The location of the alien homeworld, or their presumed "empire" of planets, is not known, so the idea of large scale offensive operations against the alien empire does not play a role in the game. The aliens do not appear to have FTL either, yet their attacks have come at varying points on the boundaries of the rough sphere of human worlds. Humanities' AIs interpret this as an (expensive) alien effort to camouflage the direction and distance of their home star systems.

Since the most rapid transfer of information possible is by vehicles using the relativistic sublight drive or using massive lasers/masers, all information about war status is long out of date when received, and similar delays are required to communicate with distant worlds.  Game play therefore resembles limited-information real time chess in which the player may only send orders and hope to receive updates.

## Time

Time in the game universe is measured throughout human space in pulsar-calibrated solar years. A 2- to 4-hour game covers some hundreds of solar years, with details to be polished from early game play. The objective is to build a game that does not require twitch responses, but rather allows the human player time for thought.

## Game Actions

Game actions consist of translating economic output into weapons construction and deployments and ordering existing forces to move from place to place. There are no economic actions in the gameplay; the economic power to produce weapons simply grows over time in each star systemas a function of the population and natural resources of star systems, which are randomly initialized before the game begins. Each star system's economy produces value which can be translated into weapons through game actions, after which those weapons that can move from system to system (some weapons systems are fixed in their locations) may be directed to move. The game is not turn-based, but rather the possible actions are limited by economic output. Within that limitation, any number of actions may be taken at any time by both the human and the bot players.

## Economic levels

Each inhabited human star system has an economic level.  Every inhabited system is capable of feeding itself and meeting its own need for materials. Every inhabited system is spacefaring within its own system. So economic levels maintained by the game engine range from 1, this basic level, allows for the construction of planetary defenses or light spacegoing intercepters over time, while the highest economic level, 5, approaches Kardashev level 2 (full use of the output from a star) which allows construction of interstellar battle fleets over time. The Earth is presumed to have economic level 5 at the start of the game.

## Possible weapons

Weapons available to the player range from the most inexpensive and least powerful to the most expensive and powerful. As noted above, every inhabited system is capable of building local orbital defensive weapons with advantages and disadvantages analogous to 20th century CIWS. "Orbital defense" is therefore the first weapon in the hierarchy of choices for play. Increasingly expensive weapons include "reporters", which are light interstellar craft carrying no weapons that serve only to automatically return to earth from an attacked system if combat occurs and report the results, "interceptors" without interstellar drives (similar to frigates in the water navy),with Interstellar escorts and interstellar battleships at the upper end of the cost scale.

## Combat

Combat occurs when alien and human forces find themselves in the same star system. The player has no role in combat; rather, combat is resolved by the game engine as a stocastic function of the types and quantities of weapons present when it occurs. The results or even the occrence of combat are not necessarily observable outside the star system in which combat occurs.  It will be reported back to players if the forces were accompanied by the appropriate machines to report the occurrence of the combat and, if possible, its results. Arranging for the presence of such reporting systems requires explicit game actions (see "reporters" under possible weapons.)

Combat occurs only within star systems, never at relativistic interstellar speeds.

## Victory conditions

Alien attacks begin to tail off after a period of play measured in the game universe, finally diminishing to none at end of play. This represents exhaustion on the part of the alien empire. Large alien losses accelerate the exhaustion point. The human side wins by minimizing losses prior to alien exhaustion and by retaining control of its star systems. The alien side wins by capturing a sufficient number (most of the...) human systems, particularly Earth. Numerical details of these conditions will be determined from game play and folded back into the software.

## Software

The game software will be as a single page application served by a small purpose-built Golang server that only serves up the page on the local host. Human space will be visualized as in the prototype found in the directory `Proto`. The prototype will be extended to display additional game state information in addition to the name of the star system in response to mouse over events. A sidebar will be added to display events that have occurred in the recent past and a way for the user to input their game actions (e.g. menus or text) will be designed. A well designed API for bots will allow substitution of bots and the possible opportunity for two player play in the future.



