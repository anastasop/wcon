
const wcon = (function() {
    const ctlProgram = function(uri) {
	fetch(uri, {method: "POST"})
	    .then(() => location.reload())
	    .catch(err => alert(err));
    };

    const startProgram = id => ctlProgram("/start/" + id);
    const stopProgram = id => ctlProgram("/stop/" + id);

    return {
	startProgram: startProgram,
	stopProgram: stopProgram
    };
})();

document.addEventListener("DOMContentLoaded", function(){
    document.querySelectorAll(".opbut.start")
	.forEach(but => but.addEventListener('click', (e) => wcon.startProgram(e.target.dataset.prog)));
    document.querySelectorAll(".opbut.stop")
	.forEach(but => but.addEventListener('click', (e) => wcon.stopProgram(e.target.dataset.pid)));
});
