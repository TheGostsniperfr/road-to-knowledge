# ADR-0005: Own WebGL2 renderer instead of a graph library

- Status: accepted, 2026-11-08

## Context
The early design used React Flow and elkjs for a node graph. The validated design is a
galaxy map: hundreds of thousands of particles, crisp at every zoom.

## Options
1. React Flow + elkjs: boxes and edges, not galaxies.
2. three.js: capable, about 600 kB for features we would not use.
3. Raw WebGL2 with a small engine, Canvas 2D overlay for labels and interaction.

## Decision
Option 3. Physics-inspired generation (density-wave orbits, exponential disk, Sérsic
bulge, Plummer clusters, blackbody colours), slot data in a float texture, nebula
shader at half resolution. Panels use Preact.
