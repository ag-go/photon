--this plugins plays the media on the selected cars on <ctrl>p
--when the player ended, moves the selected card to the right
--so user can type 10<ctrl>p and play the next 10 items
photon = require("photon")
events = require("photon.events")
keybindings = require("photon.keybindings")

run = 0

keybindings.add(photon.NormalState, "<ctrl>p", function()
	if run == 0 then 
		photon.selectedCard.runMedia()
	end
	run = run + 1
end)

events.subscribe(events.RunMediaEnd, function(e)
	if run > 1 then 
		photon.selectedCard.moveRight()
		photon.selectedCard.runMedia()
		run = run - 1
	elseif run == 1 then
		run = 0
	end
end)
