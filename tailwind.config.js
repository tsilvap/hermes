/** @type {import('tailwindcss').Config} */
module.exports = {
    content: ["./templates/*.tmpl"],
    theme: {
        fontFamily: {
            "sans": ["Cooper Hewitt", "sans-serif"],
        },
    },
    plugins: [
        require("@tailwindcss/typography"),
        require("daisyui"),
    ],
}
