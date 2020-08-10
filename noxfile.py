import os
import pathlib
import shutil

import nox


if __name__ == "__main__":
    __import__("sys").exit(
        "Do not execute this file directly. Use nox instead, it will know how to handle this file"
    )


python_paths = [
    "contracts/tests/",
    "noxfile.py",
]
requirements_as_constraints = ["-c", "requirements.txt"]

nox.options.error_on_external_run = True
nox.options.reuse_existing_virtualenvs = True


@nox.session
def update_requirements(session):
    session.install("pip-tools")
    session.run("pip-compile", "requirements.in")


@nox.session
def upgrade_requirements(session):
    session.install("pip-tools")
    session.run("pip-compile", "-U", "requirements.in")


@nox.session()
def install_ganache(session):
    """install ganache-cli"""
    session.install("nodeenv")
    nodeenv_dir = pathlib.Path(session.bin).parent.joinpath("node")
    ganache_cli = nodeenv_dir.joinpath("bin/ganache-cli")
    if not ganache_cli.exists():

        if nodeenv_dir.exists():
            shutil.rmtree(nodeenv_dir)

        session.run("nodeenv", "--node", "12.18.0", str(nodeenv_dir))

        os.environ["PATH"] = f"{nodeenv_dir}/bin" + os.pathsep + os.environ["PATH"]
        session.env["PATH"] = f"{nodeenv_dir}/bin" + os.pathsep + session.env["PATH"]

        session.run("npm", "install", "-g", "ganache-cli@6.9.1", silent=True, external=True)


@nox.session
def black(session):
    session.install("black", *requirements_as_constraints)
    session.run("black", "--check", "--diff", *python_paths)


@nox.session
def flake8(session):
    session.install("flake8", *requirements_as_constraints)
    session.run("flake8", *python_paths)


@nox.session
def zimports(session):
    session.install("zimports", *requirements_as_constraints)
    session.run("zimports", *python_paths)


@nox.session
def test_contracts(session):
    session.install("-r", "requirements.txt")
    session.chdir("contracts")
    session.run("brownie", "test")
