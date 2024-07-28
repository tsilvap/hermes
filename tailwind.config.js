/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./templates/*.tmpl"],
  plugins: [
    require("@tailwindcss/typography"),
    require("daisyui"),
  ],
}
