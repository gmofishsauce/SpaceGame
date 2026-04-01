# Adjustments to game design


This document describes some adjustments to the design of the game found in this repository. These adjustments are the result of my evolving understanding of the desired gameplay. As such they are not the result of errors in previous versions of the affected documents.
These adjustments are in no particular order. 

- Let's put a comment at the top of the architectural.md file to the effect that this document is obsolete and is retained for historical reasons. The design.md file is completely standalone and we will adjust the design there without correcting the original requirements document.

- add to FR-002/FR-004 or nearby: that in addition to serving the page, the server also presents an API on /api that serves as the authoritative source of game state information for the client.

- FR-015: I think it should say that any arrival of a command in a distant system is a logged game event. What happens in response is a separate logged game event. The command may be ignored (because the system was capture), it may be impossible to execute (because of lack of economic wealth), or it may be executed.

- FR-016: the timings given in this requirement are correct. But there is an erroneous assumption. You wrote: "standard events propagate at c".  This is of course true as far as it goes, but in fact the execution of a command is not visible over interstellar distances. Only events that are (somehow) reported are visible in the UI at Sol.  To increase the likelihood of reporting ti Sol, we will add a new "weapon" to the weapons hierarchy. This weapon or really device is an interstellar communication laser. It is expensive (see updates to weapons hierarchy below). If there is such a device in a stellar system, all logged events are reported to Sol and arrive at light speed.

- FR-017 should read:	The player UI only shows **reported** events whose arrivalYear ≤ currentGameYear. The central game log should record whether an event could be be reported when it occurred in the local frame of reference (true if either (1) a reporter vehicle escaped the system into relativistic flight or (2) the system contained an interstellar comm laser at the time of event occurrence in the local frame).

In section 2.7:

- All command events originate from Sol. Other star systems do not need active context menus. All star systems do need active right click status displays that display "best information" (status based on arrived events at Sol).

In section 2.10:


- FR-040: Add a sixth weapon type, "Comm laser", which has no combat value but ensures reporting at light speed back to Sol. Also, comm lasers always report at least the arrival of aliens, even if they are later destroyed in combat, while reporters can be destroyed and fail to report anything bacl to Sol.


In section 2.11:

- Each stellar system has a concept of "wealth". Wealth accumulates at a rate determined by the system's economic level. The accumulation rate is an exponential function of the economic level. A sysytem with economic level N (0 <= N <= 5)accumulates wealth at 2^N units per game year.


Economic level rises 1 level per 100 game years in absence of combat. Any combat in a system drops the system's economic level by 1 and resets the 100 year growth clock.

**Weaons costs:**

- Comm laser:  64 units of wealth
- Battleship: 32 units 
- Escort: 8 units
- Reporter: 4 units
- interceptor 2 units
- orbital defense: 1 unit








