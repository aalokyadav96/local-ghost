import { createNav } from "./navigation.js";
import { displayAuthSection, displayUserProfile } from "./main.js";
import { displayFeed } from "./feed.js";

async function loadContent(url) {
    const app = document.getElementById("app");
    const path = window.location.pathname;

    app.innerHTML = createNav() + `<div id="content"></div>`;
    const content = document.getElementById("content");

    // You can dynamically load content based on the URL (SPA-style)
    switch (url) {
        case '/':
            content.innerHTML = `<h1>Welcome to the App<div id="suggested"></div></h1>`;
            displayFeed();
            break;
        case '/login':
            content.innerHTML = `<div id="auth-section"></div>`;
            displayAuthSection();
            break;
        default:
            if (path.startsWith('/user/') && path.length > 6) {
                const username = path.split('/')[2];
                await displayUserProfile(username);
            } else {
                content.innerHTML = `<h1>404 Not Found</h1>`;
            }
    }
}

function navigate(url) {
    // Update the URL without reloading the page
    history.pushState(null, '', url);

    // Load the content for the new URL
    loadContent(url);
}


export { renderPage, navigate, loadContent };